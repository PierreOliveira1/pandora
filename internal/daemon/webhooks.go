package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/webhook"
)

type WebhooksState struct {
	Listening bool   `json:"listening"`
	Address   string `json:"address,omitempty"`
	Readers   int    `json:"readers"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleWebhooksReload(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.reloadWebhooks(r.Context()))
}

func (s *Server) reloadWebhooks(ctx context.Context) WebhooksState {
	s.webhooksMu.Lock()
	defer s.webhooksMu.Unlock()

	active, port, err := s.core.WebhooksActive()
	if err != nil {
		s.active.Store(false)
		slog.Error("checking webhooks", "error", err)
		return WebhooksState{Error: err.Error()}
	}
	s.active.Store(active)

	state := WebhooksState{}
	if active {
		if port == 0 {
			port, err = s.core.EnsureWebhookPort(ctx)
			if err != nil {
				state.Error = err.Error()
				slog.Error("finding the webhook port", "error", err)
			}
		}
		if state.Error == "" {
			if err := s.startReceiverLocked(port); err != nil {
				state.Error = err.Error()
				slog.Error("starting the webhook receiver", "port", port, "error", err)
			} else {
				state.Listening = true
				state.Address = fmt.Sprintf("127.0.0.1:%d", port)
			}
		}
	} else {
		s.stopReceiverLocked()
	}

	readers, err := s.core.EventReaders()
	if err != nil {
		state.Error = joinErrors(state.Error, err.Error())
		slog.Error("loading event readers", "error", err)
	} else {
		s.reconcileReadersLocked(readers)
	}
	state.Readers = len(s.readers)
	return state
}

func (s *Server) startReceiverLocked(port int) error {
	if port <= 0 {
		return errors.New("the webhook receiver has no port")
	}
	if s.receiver != nil && s.receiverPort == port {
		return nil
	}
	s.stopReceiverLocked()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		if webhookPortInUse(err) {
			return fmt.Errorf("port %d is in use: another program is using it", port)
		}
		return fmt.Errorf("listening for webhooks on %s: %w", address, err)
	}
	server := &http.Server{
		Handler:           webhook.NewHandler(s.core.WebhookLookup, s.core.Events()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.receiver = server
	s.receiverPort = port
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("webhook receiver stopped", "address", address, "error", err)
		}
	}()
	return nil
}

func (s *Server) stopReceiverLocked() {
	if s.receiver == nil {
		s.receiverPort = 0
		return
	}
	server := s.receiver
	s.receiver = nil
	s.receiverPort = 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("stopping the webhook receiver", "error", err)
	}
}

func (s *Server) reconcileReadersLocked(readers []config.EventReader) {
	if s.readers == nil {
		s.readers = map[string]*runningReader{}
	}
	desired := make(map[string]config.EventReader, len(readers))
	for _, reader := range readers {
		desired[reader.Name] = reader
	}
	for name, running := range s.readers {
		next, ok := desired[name]
		if ok && reflect.DeepEqual(next, running.config) {
			continue
		}
		running.cancel()
		delete(s.readers, name)
		<-running.done
	}
	for name, reader := range desired {
		if _, ok := s.readers[name]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		running := &runningReader{
			cancel: cancel,
			done:   make(chan struct{}),
			config: reader,
		}
		s.readers[name] = running
		go func(reader config.EventReader, running *runningReader, ctx context.Context) {
			defer close(running.done)
			s.runReader(ctx, reader)
		}(reader, running, ctx)
	}
}

func (s *Server) stopReadersLocked() {
	for name, running := range s.readers {
		running.cancel()
		delete(s.readers, name)
		<-running.done
	}
}

func (s *Server) stopWebhooks() {
	s.webhooksMu.Lock()
	defer s.webhooksMu.Unlock()
	s.stopReceiverLocked()
	s.stopReadersLocked()
	s.active.Store(false)
}

func (s *Server) restoreTunnel(ctx context.Context) {
	info, err := s.core.TunnelStatus(ctx)
	if err != nil {
		slog.Error("checking the webhook tunnel", "error", err)
		return
	}
	if info.Provider == "" || info.Exposed {
		return
	}
	restored, err := s.core.UseTunnel(ctx, info.Provider, info.PublicURL)
	if err != nil {
		slog.Error("restoring the webhook tunnel", "provider", info.Provider, "error", err)
		return
	}
	slog.Info("webhook tunnel restored", "provider", restored.Provider, "public_url", restored.PublicURL)
}

func joinErrors(first, second string) string {
	if first == "" {
		return second
	}
	return first + "; " + second
}

func webhookPortInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address already in use") ||
		strings.Contains(message, "only one usage of each socket address")
}
