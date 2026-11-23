package core

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/source"
)

const statusTimeout = 15 * time.Second

type StatusInfo struct {
	Ref        string        `json:"ref"`
	OK         bool          `json:"ok"`
	NeedsLogin bool          `json:"needs_login"`
	Tools      int           `json:"tools"`
	Latency    time.Duration `json:"latency_ms"`
	Error      string        `json:"error,omitempty"`
}

func (s StatusInfo) MarshalJSON() ([]byte, error) {
	type statusJSON struct {
		Ref        string `json:"ref"`
		OK         bool   `json:"ok"`
		NeedsLogin bool   `json:"needs_login"`
		Tools      int    `json:"tools"`
		LatencyMS  int64  `json:"latency_ms"`
		Error      string `json:"error,omitempty"`
	}
	return json.Marshal(statusJSON{
		Ref:        s.Ref,
		OK:         s.OK,
		NeedsLogin: s.NeedsLogin,
		Tools:      s.Tools,
		LatencyMS:  s.Latency.Milliseconds(),
		Error:      s.Error,
	})
}

func (c *Core) Status(ctx context.Context, ref string) StatusInfo {
	start := time.Now()
	info := StatusInfo{Ref: ref}

	if cfg, err := c.store.Load(); err == nil {
		if parsed, err := registry.ParseRef(ref); err == nil {
			if resolved, err := registry.Resolve(cfg, parsed); err == nil {
				info.Ref = resolved.String()
			}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()

	conn, err := c.open(ctx, ref)
	if err != nil {
		info.Latency = time.Since(start)
		var authErr *source.AuthRequiredError
		if errors.As(err, &authErr) {
			info.NeedsLogin = true
		}
		info.Error = err.Error()
		return info
	}
	info.Ref = conn.key

	tools, err := conn.src.ListTools(ctx)
	info.Latency = time.Since(start)
	if err != nil {
		if isConnectionError(err) {
			c.drop(conn.key)
		}
		var authErr *source.AuthRequiredError
		if errors.As(err, &authErr) {
			info.NeedsLogin = true
		}
		info.Error = err.Error()
		return info
	}

	info.OK = true
	info.Tools = len(tools)
	return info
}

func (c *Core) StatusAll(ctx context.Context) []StatusInfo {
	refs, err := c.accountRefs()
	if err != nil {
		return []StatusInfo{}
	}

	results := make([]StatusInfo, len(refs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Add(1)
		go func(i int, ref string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() {
				<-sem
			}()
			results[i] = c.Status(ctx, ref)
		}(i, ref)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].Ref < results[j].Ref
	})
	return results
}

func (c *Core) accountRefs() ([]string, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	refs := make([]string, 0)
	for service, value := range cfg.Services {
		for account := range value.Accounts {
			refs = append(refs, service+"/"+account)
		}
	}
	sort.Strings(refs)
	return refs, nil
}
