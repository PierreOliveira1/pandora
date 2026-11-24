package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/oauth"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

type Options struct {
	Store      *config.Store
	Secrets    secrets.Store
	Openers    source.Openers
	Version    string
	InstallEnv install.Env
	OAuth      *oauth.Manager
	CallLog    *guard.Log
	Tunnel     func(settings config.WebhookSettings) tunnel.Provider
}

type Core struct {
	store      *config.Store
	secrets    secrets.Store
	openers    source.Openers
	version    string
	installEnv install.Env
	oauth      *oauth.Manager
	callLog    *guard.Log
	httpClient *http.Client
	events     *events.Queue
	tunnel     func(settings config.WebhookSettings) tunnel.Provider
	mu         sync.Mutex
	pool       map[string]*pooledSource
	inflight   map[string]*pendingOpen
}

type Catalog struct {
	FetchedAt    time.Time   `json:"fetched_at"`
	Instructions string      `json:"instructions,omitempty"`
	Tools        []*mcp.Tool `json:"tools"`
}

type SearchHit struct {
	Service string `json:"service"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

type connection struct {
	key string
	src source.Source
}

type pooledSource struct {
	src         source.Source
	fingerprint string
	lastUsed    time.Time
}

type pendingOpen struct {
	done chan struct{}
	src  source.Source
	err  error
}

func New(o Options) *Core {
	openers := o.Openers
	if openers == nil {
		openers = source.Openers{}
	}
	return &Core{
		store:      o.Store,
		secrets:    o.Secrets,
		openers:    openers,
		version:    o.Version,
		installEnv: o.InstallEnv,
		oauth:      o.OAuth,
		callLog:    o.CallLog,
		httpClient: &http.Client{},
		events:     events.NewQueue(o.Store.Paths().EventsDir(), time.Now),
		tunnel:     o.Tunnel,
		pool:       map[string]*pooledSource{},
		inflight:   map[string]*pendingOpen{},
	}
}

func (c *Core) Tools(ctx context.Context, service string, refresh bool) (*Catalog, error) {
	cached, cacheErr := c.loadCatalog(service)
	if cacheErr == nil && !refresh && time.Since(cached.FetchedAt) < catalogTTL {
		return cached, nil
	}

	conn, err := c.open(ctx, service)
	if err != nil {
		if cacheErr == nil {
			return cached, nil
		}
		return nil, err
	}
	defer c.touch(conn.key)
	tools, err := conn.src.ListTools(ctx)
	if err != nil {
		if isConnectionError(err) {
			c.drop(conn.key)
		}
		if cacheErr == nil {
			return cached, nil
		}
		return nil, fmt.Errorf("listing tools of %s: %w", conn.key, err)
	}
	if tools == nil {
		tools = []*mcp.Tool{}
	}

	catalog := &Catalog{
		FetchedAt:    time.Now(),
		Instructions: conn.src.Instructions(),
		Tools:        tools,
	}
	_ = c.saveCatalog(service, catalog)
	return catalog, nil
}

func (c *Core) Describe(ctx context.Context, service, toolName string) (*mcp.Tool, error) {
	catalog, err := c.Tools(ctx, service, false)
	if err != nil {
		return nil, err
	}
	for _, tool := range catalog.Tools {
		if tool.Name == toolName {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("unknown tool %q in service %q: run pandora tools %s", toolName, service, service)
}

func (c *Core) Search(ctx context.Context, query, service string, limit int) ([]SearchHit, int, error) {
	services, err := c.searchServices(service)
	if err != nil {
		return nil, 0, err
	}

	words := strings.Fields(strings.ToLower(query))
	hits := make([]SearchHit, 0)
	for _, name := range services {
		catalog, err := c.Tools(ctx, name, false)
		if err != nil {
			continue
		}
		for _, tool := range catalog.Tools {
			haystack := strings.ToLower(tool.Name + "\n" + tool.Description)
			if !matchesAll(haystack, words) {
				continue
			}
			hits = append(hits, SearchHit{
				Service: name,
				Tool:    tool.Name,
				Summary: firstLine(tool.Description),
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Service != hits[j].Service {
			return hits[i].Service < hits[j].Service
		}
		return hits[i].Tool < hits[j].Tool
	})
	total := len(hits)
	if limit <= 0 {
		limit = 20
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, total, nil
}

func (c *Core) Call(ctx context.Context, ref string, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	started := time.Now()
	logRef := ref
	result, err := func() (*mcp.CallToolResult, error) {
		resolved, _, account, err := c.resolve(ref)
		if err != nil {
			return nil, err
		}
		logRef = resolved.String()
		if err := c.checkPolicy(ctx, resolved, account, tool); err != nil {
			return nil, err
		}
		return c.callTool(ctx, logRef, tool, args)
	}()
	c.recordCall(ctx, logRef, tool, started, result, err)
	return result, err
}

func (c *Core) callTool(ctx context.Context, ref string, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	conn, err := c.open(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer c.touch(conn.key)
	result, err := conn.src.CallTool(ctx, tool, args)
	if err != nil {
		var authErr *source.AuthRequiredError
		if errors.As(err, &authErr) {
			c.drop(conn.key)
			return nil, err
		}
		if isConnectionError(err) {
			c.drop(conn.key)
		}
		return nil, fmt.Errorf("calling tool %q on %s: %w", tool, conn.key, err)
	}
	return result, nil
}

func (c *Core) Close() error {
	c.mu.Lock()
	pool := c.pool
	c.pool = map[string]*pooledSource{}
	c.mu.Unlock()

	keys := make([]string, 0, len(pool))
	for key := range pool {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var errs []error
	for _, key := range keys {
		if err := pool[key].src.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Core) Connections() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pool)
}

func (c *Core) StartJanitor(ctx context.Context, idle time.Duration) {
	if idle <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				c.closeIdle(now.Add(-idle))
			}
		}
	}()
}

func (c *Core) closeIdle(cutoff time.Time) {
	c.mu.Lock()
	var closing []source.Source
	for key, entry := range c.pool {
		if entry.lastUsed.Before(cutoff) {
			delete(c.pool, key)
			closing = append(closing, entry.src)
		}
	}
	c.mu.Unlock()
	for _, src := range closing {
		_ = src.Close()
	}
}

func (c *Core) resolve(ref string) (registry.Ref, *config.Service, *config.Account, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return registry.Ref{}, nil, nil, err
	}
	parsed, err := registry.ParseRef(ref)
	if err != nil {
		return registry.Ref{}, nil, nil, err
	}
	resolved, err := registry.Resolve(cfg, parsed)
	if err != nil {
		return registry.Ref{}, nil, nil, err
	}
	service := cfg.Services[resolved.Service]
	return resolved, service, service.Accounts[resolved.Account], nil
}

func (c *Core) open(ctx context.Context, ref string) (*connection, error) {
	resolved, service, account, err := c.resolve(ref)
	if err != nil {
		return nil, err
	}
	key := resolved.String()

	opener, ok := c.openers[service.Kind]
	if !ok {
		return nil, fmt.Errorf("kind %s is not supported yet", service.Kind)
	}
	fingerprint := accountFingerprint(service, account)

	for {
		c.mu.Lock()
		if entry, ok := c.pool[key]; ok {
			if entry.fingerprint == fingerprint {
				entry.lastUsed = time.Now()
				src := entry.src
				c.mu.Unlock()
				return &connection{key: key, src: src}, nil
			}
			delete(c.pool, key)
			c.mu.Unlock()
			_ = entry.src.Close()
			continue
		}
		if pending, ok := c.inflight[key]; ok {
			c.mu.Unlock()
			if _, err := waitForOpen(ctx, key, pending); err != nil {
				return nil, err
			}
			continue
		}
		pending := &pendingOpen{done: make(chan struct{})}
		c.inflight[key] = pending
		c.mu.Unlock()

		spec, err := c.buildSpec(resolved, service, account)
		if err != nil {
			pending.err = err
		} else {
			pending.src, pending.err = opener(ctx, spec)
		}

		c.mu.Lock()
		delete(c.inflight, key)
		if pending.err == nil {
			c.pool[key] = &pooledSource{
				src:         pending.src,
				fingerprint: fingerprint,
				lastUsed:    time.Now(),
			}
		}
		c.mu.Unlock()
		close(pending.done)

		if pending.err != nil {
			return nil, pending.err
		}
		return &connection{key: key, src: pending.src}, nil
	}
}

func waitForOpen(ctx context.Context, key string, pending *pendingOpen) (*connection, error) {
	select {
	case <-pending.done:
		if pending.err != nil {
			return nil, pending.err
		}
		return &connection{key: key, src: pending.src}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for %s: %w", key, ctx.Err())
	}
}

func (c *Core) buildSpec(ref registry.Ref, service *config.Service, account *config.Account) (source.Spec, error) {
	url := account.URL
	if url == "" {
		url = service.URL
	}

	env := make(map[string]string, len(service.Env)+len(account.Env)+len(account.SecretEnv))
	for name, value := range service.Env {
		env[name] = value
	}
	for name, value := range account.Env {
		env[name] = value
	}
	for _, name := range account.SecretEnv {
		value, err := c.secrets.Get(secrets.EnvKey(ref.Service, ref.Account, name))
		if errors.Is(err, secrets.ErrNotFound) {
			return source.Spec{}, missingSecret(name, ref.String())
		}
		if err != nil {
			return source.Spec{}, fmt.Errorf("reading secret %s for %s: %w", name, ref, err)
		}
		env[name] = value
	}

	headers := make(map[string]string, len(account.SecretHeaders))
	for _, name := range account.SecretHeaders {
		value, err := c.secrets.Get(secrets.HeaderKey(ref.Service, ref.Account, name))
		if errors.Is(err, secrets.ErrNotFound) {
			return source.Spec{}, missingSecret(name, ref.String())
		}
		if err != nil {
			return source.Spec{}, fmt.Errorf("reading secret %s for %s: %w", name, ref, err)
		}
		headers[http.CanonicalHeaderKey(name)] = value
	}

	var token, refresh func(ctx context.Context) (string, error)
	if service.Auth == config.AuthOAuth && c.oauth != nil {
		token = func(ctx context.Context) (string, error) {
			value, err := c.oauth.AccessToken(ctx, ref.Service, ref.Account)
			if errors.Is(err, oauth.ErrLoginRequired) {
				return "", &source.AuthRequiredError{Ref: ref.String()}
			}
			return value, err
		}
		refresh = func(ctx context.Context) (string, error) {
			value, err := c.oauth.ForceRefresh(ctx, ref.Service, ref.Account)
			if errors.Is(err, oauth.ErrLoginRequired) {
				return "", &source.AuthRequiredError{Ref: ref.String()}
			}
			return value, err
		}
	}

	return source.Spec{
		Service:    ref.Service,
		Account:    ref.Account,
		Kind:       service.Kind,
		Svc:        service,
		Acc:        account,
		URL:        url,
		Env:        env,
		Headers:    headers,
		Token:      token,
		Refresh:    refresh,
		HTTPClient: c.httpClient,
		LogPath:    c.store.Paths().UpstreamLog(ref.Service, ref.Account),
	}, nil
}

func (c *Core) drop(key string) {
	c.mu.Lock()
	entry, ok := c.pool[key]
	delete(c.pool, key)
	c.mu.Unlock()
	if ok {
		_ = entry.src.Close()
	}
}

func (c *Core) touch(key string) {
	c.mu.Lock()
	if entry, ok := c.pool[key]; ok {
		entry.lastUsed = time.Now()
	}
	c.mu.Unlock()
}

func (c *Core) searchServices(service string) ([]string, error) {
	if service != "" {
		return []string{service}, nil
	}
	cfg, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	services := make([]string, 0, len(cfg.Services))
	for name, value := range cfg.Services {
		if len(value.Accounts) > 0 {
			services = append(services, name)
		}
	}
	sort.Strings(services)
	return services, nil
}

func matchesAll(haystack string, words []string) bool {
	for _, word := range words {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

func missingSecret(name, ref string) error {
	return fmt.Errorf("missing secret %s for %s: run pandora remove %s and add it again", name, ref, ref)
}

func isConnectionError(err error) bool {
	return errors.Is(err, mcp.ErrConnectionClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET)
}
