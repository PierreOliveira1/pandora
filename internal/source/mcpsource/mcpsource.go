package mcpsource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/oauth"
	"github.com/PierreOliveira1/pandora/internal/source"
)

const clientVersion = "0.1.0-dev"

type session struct {
	session *mcp.ClientSession
	logFile *os.File
}

func (s *session) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	tools := make([]*mcp.Tool, 0)
	for tool, err := range s.session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (s *session) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	return s.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}

func (s *session) Instructions() string {
	result := s.session.InitializeResult()
	if result == nil {
		return ""
	}
	return result.Instructions
}

func (s *session) Close() error {
	err := s.session.Close()
	if s.logFile != nil {
		if closeErr := s.logFile.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func OpenStdio(ctx context.Context, spec source.Spec) (source.Source, error) {
	if spec.Svc == nil || len(spec.Svc.Command) == 0 {
		return nil, fmt.Errorf("service %q has no command", spec.Service)
	}

	cmd := exec.Command(spec.Svc.Command[0], spec.Svc.Command[1:]...)
	cmd.Env = buildEnvironment(spec.Env)

	var logFile *os.File
	if spec.LogPath != "" {
		var err error
		logFile, err = openLog(spec.LogPath)
		if err != nil {
			return nil, err
		}
		cmd.Stderr = logFile
	}

	clientSession, err := newClient().Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, fmt.Errorf("connecting to %s: %w", refOf(spec), err)
	}
	return &session{session: clientSession, logFile: logFile}, nil
}

func OpenHTTP(ctx context.Context, spec source.Spec) (source.Source, error) {
	ref := refOf(spec)
	var authErr *source.AuthRequiredError

	baseTransport := http.RoundTripper(http.DefaultTransport)
	if spec.HTTPClient != nil && spec.HTTPClient.Transport != nil {
		baseTransport = spec.HTTPClient.Transport
	}
	if spec.Svc != nil && spec.Svc.Auth == config.AuthOAuth {
		if spec.Token == nil {
			return nil, &source.AuthRequiredError{Ref: ref}
		}
		if _, err := spec.Token(ctx); err != nil {
			if errors.As(err, &authErr) {
				return nil, authErr
			}
			return nil, fmt.Errorf("getting OAuth token for %s: %w", ref, err)
		}
		baseTransport = oauth.NewTransport(baseTransport, spec.Token, spec.Refresh)
	}

	client, transport := newHTTPClient(spec.HTTPClient, spec.Headers, baseTransport)
	clientSession, firstErr := connectHTTP(ctx, &mcp.StreamableClientTransport{
		Endpoint:   spec.URL,
		HTTPClient: client,
	})
	if firstErr == nil {
		return &session{session: clientSession}, nil
	}
	if errors.As(firstErr, &authErr) {
		return nil, authErr
	}
	if transport.unauthorized.Load() {
		return nil, &source.AuthRequiredError{Ref: ref}
	}

	clientSession, sseErr := connectHTTP(ctx, &mcp.SSEClientTransport{
		Endpoint:   spec.URL,
		HTTPClient: client,
	})
	if sseErr == nil {
		return &session{session: clientSession}, nil
	}
	if errors.As(sseErr, &authErr) {
		return nil, authErr
	}
	if transport.unauthorized.Load() {
		return nil, &source.AuthRequiredError{Ref: ref}
	}
	return nil, fmt.Errorf("connecting to %s: %w", ref, errors.Join(firstErr, sseErr))
}

func Register(openers source.Openers) {
	openers[config.KindMCPStdio] = OpenStdio
	openers[config.KindMCPHTTP] = OpenHTTP
}

func connectHTTP(ctx context.Context, transport mcp.Transport) (*mcp.ClientSession, error) {
	return newClient().Connect(ctx, transport, nil)
}

func newClient() *mcp.Client {
	return mcp.NewClient(&mcp.Implementation{Name: "pandora", Version: clientVersion}, nil)
}

func refOf(spec source.Spec) string {
	return spec.Service + "/" + spec.Account
}

func buildEnvironment(extra map[string]string) []string {
	values := make(map[string]string)
	order := make([]string, 0, len(extra))
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, exists := values[name]; !exists {
			order = append(order, name)
		}
		values[name] = value
	}

	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, exists := values[name]; !exists {
			order = append(order, name)
		}
		values[name] = extra[name]
	}

	env := make([]string, 0, len(order))
	for _, name := range order {
		env = append(env, name+"="+values[name])
	}
	return env
}

func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating upstream log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening upstream log: %w", err)
	}
	return file, nil
}

type headerTransport struct {
	base         http.RoundTripper
	headers      map[string]string
	unauthorized atomic.Bool
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	for name, value := range t.headers {
		clone.Header.Set(name, value)
	}

	resp, err := t.base.RoundTrip(clone)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		t.unauthorized.Store(true)
	}
	return resp, err
}

func newHTTPClient(base *http.Client, headers map[string]string, baseTransport http.RoundTripper) (*http.Client, *headerTransport) {
	if base == nil {
		base = http.DefaultClient
	}
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	transport := &headerTransport{
		base:    baseTransport,
		headers: headers,
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       base.Timeout,
	}, transport
}
