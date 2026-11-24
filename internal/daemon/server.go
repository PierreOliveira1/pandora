package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/flock"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/source"
)

var ErrAlreadyRunning = errors.New("daemon already running")

type Health struct {
	PID         int       `json:"pid"`
	Version     string    `json:"version"`
	StartedAt   time.Time `json:"started_at"`
	Connections int       `json:"connections"`
}

type Server struct {
	core          *core.Core
	version       string
	idle          time.Duration
	startedAt     time.Time
	mu            sync.Mutex
	lastActivity  time.Time
	inflight      int
	shutdown      chan struct{}
	shutdownOnce  sync.Once
	webhooksMu    sync.Mutex
	receiver      *http.Server
	receiverPort  int
	active        atomic.Bool
	readers       map[string]*runningReader
	readerBackoff []time.Duration
	reloadEvery   time.Duration
}

func NewServer(c *core.Core, version string, idle time.Duration) *Server {
	return &Server{
		core:          c,
		version:       version,
		idle:          idle,
		startedAt:     time.Now(),
		shutdown:      make(chan struct{}),
		readers:       map[string]*runningReader{},
		readerBackoff: []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute},
		reloadEvery:   30 * time.Second,
	}
}

type errorBody struct {
	Message    string `json:"message"`
	NeedsLogin bool   `json:"needs_login,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Blocked    bool   `json:"blocked,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type callRequest struct {
	Ref    string         `json:"ref"`
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args,omitempty"`
	Client string         `json:"client,omitempty"`
}

type callResponse struct {
	Result any `json:"result"`
}

type toolsRequest struct {
	Service string `json:"service"`
	Refresh bool   `json:"refresh"`
}

type describeRequest struct {
	Service string `json:"service"`
	Tool    string `json:"tool"`
}

type searchRequest struct {
	Query   string `json:"query"`
	Service string `json:"service"`
	Limit   int    `json:"limit"`
}

type searchResponse struct {
	Results []core.SearchHit `json:"results"`
	Total   int              `json:"total"`
}

type statusRequest struct {
	Ref string `json:"ref"`
}

func (s *Server) Serve(ctx context.Context, socketPath string) error {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating daemon socket directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("setting daemon socket directory permissions: %w", err)
	}
	lock := flock.New(socketPath + ".lock")
	locked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("locking daemon socket: %w", err)
	}
	if !locked {
		return ErrAlreadyRunning
	}
	defer lock.Unlock()
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale daemon socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", socketPath, err)
	}
	defer os.Remove(socketPath)
	if err := restrictSocket(socketPath); err != nil {
		_ = listener.Close()
		return fmt.Errorf("setting daemon socket permissions: %w", err)
	}

	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()

	janitorCtx, stopJanitor := context.WithCancel(ctx)
	s.core.StartJanitor(janitorCtx, 10*time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/call", s.handleCall)
	mux.HandleFunc("POST /v1/tools", s.handleTools)
	mux.HandleFunc("POST /v1/describe", s.handleDescribe)
	mux.HandleFunc("POST /v1/search", s.handleSearch)
	mux.HandleFunc("POST /v1/status", s.handleStatus)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/shutdown", s.handleShutdown)
	mux.HandleFunc("POST /v1/webhooks/reload", s.handleWebhooksReload)

	httpServer := &http.Server{
		Handler:           s.track(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.reloadWebhooks(ctx)
	tunnelCtx, stopTunnel := context.WithCancel(ctx)
	var tunnelWait sync.WaitGroup
	tunnelWait.Add(1)
	go func() {
		defer tunnelWait.Done()
		s.restoreTunnel(tunnelCtx)
	}()
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- httpServer.Serve(listener)
	}()

	var reloadTicker *time.Ticker
	var reloadC <-chan time.Time
	if s.reloadEvery > 0 {
		reloadTicker = time.NewTicker(s.reloadEvery)
		reloadC = reloadTicker.C
		defer reloadTicker.Stop()
	}
	pruneTicker := time.NewTicker(10 * time.Minute)
	defer pruneTicker.Stop()

	var ticker *time.Ticker
	var idleC <-chan time.Time
	if s.idle > 0 {
		interval := s.idle / 2
		if interval < 50*time.Millisecond {
			interval = 50 * time.Millisecond
		}
		if interval > time.Second {
			interval = time.Second
		}
		ticker = time.NewTicker(interval)
		idleC = ticker.C
		defer ticker.Stop()
	}

	slog.Info("pandora daemon listening", "socket", socketPath, "pid", os.Getpid(), "version", s.version)
	served := false
	var serveErr error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-s.shutdown:
			break loop
		case err := <-serveDone:
			served = true
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr = err
			}
			break loop
		case <-reloadC:
			s.reloadWebhooks(ctx)
		case <-pruneTicker.C:
			if err := s.core.Events().Prune(ctx, 7*24*time.Hour, 10000); err != nil {
				slog.Error("pruning webhook events", "error", err)
			}
		case <-idleC:
			if s.idleExceeded() {
				break loop
			}
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := httpServer.Shutdown(shutdownCtx); err != nil && serveErr == nil {
		serveErr = err
	}
	cancel()
	if !served {
		select {
		case err := <-serveDone:
			if err != nil && !errors.Is(err, http.ErrServerClosed) && serveErr == nil {
				serveErr = err
			}
		case <-time.After(time.Second):
		}
	}
	s.stopWebhooks()
	stopTunnel()
	tunnelWait.Wait()

	stopJanitor()
	_ = s.core.Close()
	slog.Info("pandora daemon stopped", "pid", os.Getpid())
	return serveErr
}

func (s *Server) idleExceeded() bool {
	if s.active.Load() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inflight == 0 && time.Since(s.lastActivity) >= s.idle
}

func (s *Server) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.inflight++
		s.lastActivity = time.Now()
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.inflight--
			s.lastActivity = time.Now()
			s.mu.Unlock()
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Health{
		PID:         os.Getpid(),
		Version:     s.version,
		StartedAt:   s.startedAt,
		Connections: s.core.Connections(),
	})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	s.shutdownOnce.Do(func() {
		close(s.shutdown)
	})
}

func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	var body callRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	ctx := core.WithClient(r.Context(), body.Client)
	result, err := s.core.Call(ctx, body.Ref, body.Tool, body.Args)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, callResponse{Result: result})
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	var body toolsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	catalog, err := s.core.Tools(r.Context(), body.Service, body.Refresh)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleDescribe(w http.ResponseWriter, r *http.Request) {
	var body describeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	tool, err := s.core.Describe(r.Context(), body.Service, body.Tool)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tool)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var body searchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	results, total, err := s.core.Search(r.Context(), body.Query, body.Service, body.Limit)
	if err != nil {
		writeBusinessError(w, err)
		return
	}
	if results == nil {
		results = []core.SearchHit{}
	}
	writeJSON(w, http.StatusOK, searchResponse{Results: results, Total: total})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var body statusRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	var infos []core.StatusInfo
	if body.Ref == "" {
		infos = s.core.StatusAll(r.Context())
	} else {
		infos = []core.StatusInfo{s.core.Status(r.Context(), body.Ref)}
	}
	if infos == nil {
		infos = []core.StatusInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeBadRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, errorEnvelope{Error: errorBody{Message: err.Error()}})
}

func writeBusinessError(w http.ResponseWriter, err error) {
	body := errorBody{Message: err.Error()}
	var authErr *source.AuthRequiredError
	if errors.As(err, &authErr) {
		body.NeedsLogin = true
		body.Ref = authErr.Ref
	}
	var blocked *core.BlockedError
	if errors.As(err, &blocked) {
		body.Blocked = true
		body.Message = blocked.Reason
	}
	writeJSON(w, http.StatusOK, errorEnvelope{Error: body})
}
