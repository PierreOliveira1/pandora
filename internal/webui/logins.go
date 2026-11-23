package webui

import (
	"context"
	"time"
)

type loginState struct {
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	Warning    string `json:"warning,omitempty"`
	finishedAt time.Time
}

type loginResponse struct {
	State   string `json:"state"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}

func (s *Server) startLogin(ref string) string {
	id := randomToken()
	s.loginsMu.Lock()
	s.cleanupLoginsLocked(time.Now())
	s.logins[id] = &loginState{State: "pending"}
	s.loginsMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		loginErr := s.core.Login(ctx, ref)
		warning := ""
		if loginErr == nil {
			warning = s.syncWarning(ctx)
		}
		s.finishLogin(id, loginErr, warning)
	}()
	return id
}

func (s *Server) finishLogin(id string, loginErr error, warning string) {
	s.loginsMu.Lock()
	defer s.loginsMu.Unlock()
	state, ok := s.logins[id]
	if !ok {
		return
	}
	state.finishedAt = time.Now()
	if loginErr != nil {
		state.State = "error"
		state.Error = loginErr.Error()
		return
	}
	state.State = "done"
	state.Warning = warning
}

func (s *Server) loginStatus(id string) (loginResponse, bool) {
	s.loginsMu.Lock()
	defer s.loginsMu.Unlock()
	s.cleanupLoginsLocked(time.Now())
	state, ok := s.logins[id]
	if !ok {
		return loginResponse{}, false
	}
	return loginResponse{State: state.State, Error: state.Error, Warning: state.Warning}, true
}

func (s *Server) cleanupLoginsLocked(now time.Time) {
	for id, state := range s.logins {
		if !state.finishedAt.IsZero() && now.Sub(state.finishedAt) > 10*time.Minute {
			delete(s.logins, id)
		}
	}
}
