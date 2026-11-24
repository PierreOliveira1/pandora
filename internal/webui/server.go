package webui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

type AutostartControl interface {
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
	Enabled(ctx context.Context) (bool, error)
}

type Options struct {
	Core           *core.Core
	Registry       *registry.Registry
	CallLog        *guard.Log
	Version        string
	Static         fs.FS
	Token          string
	ReloadWebhooks func(ctx context.Context)
	Autostart      func() (AutostartControl, error)
}

type Server struct {
	core           *core.Core
	registry       *registry.Registry
	callLog        *guard.Log
	version        string
	static         fs.FS
	token          string
	reloadWebhooks func(ctx context.Context)
	autostart      func() (AutostartControl, error)

	loginsMu sync.Mutex
	logins   map[string]*loginState
}

func New(o Options) *Server {
	token := o.Token
	if token == "" {
		token = randomToken()
	}
	static := o.Static
	if static == nil {
		static = defaultStatic()
	}
	return &Server{
		core:           o.Core,
		registry:       o.Registry,
		callLog:        o.CallLog,
		version:        o.Version,
		static:         static,
		token:          token,
		reloadWebhooks: o.ReloadWebhooks,
		autostart:      o.Autostart,
		logins:         map[string]*loginState{},
	}
}

func (s *Server) Handler(addr string) http.Handler {
	return s.security(addr, s.routes())
}

func (s *Server) Start(ctx context.Context, port int) (string, <-chan error, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", nil, fmt.Errorf("starting Pandora UI on 127.0.0.1:%d: %w", port, err)
	}
	addr := listener.Addr().String()
	httpServer := &http.Server{
		Handler:           s.Handler(addr),
		ReadHeaderTimeout: 10 * time.Second,
	}
	done := make(chan error, 1)
	serveDone := make(chan struct{})
	go func() {
		defer close(done)
		serveErr := httpServer.Serve(listener)
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		done <- serveErr
		close(serveDone)
	}()
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
		case <-serveDone:
		}
	}()
	tcpAddr := listener.Addr().(*net.TCPAddr)
	startURL := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", tcpAddr.Port, url.QueryEscape(s.token))
	return startURL, done, nil
}

func randomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("generating Pandora UI token: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(value[:])
}
