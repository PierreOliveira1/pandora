package webui

import (
	"net/http"
	"runtime"
	"strconv"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

const autostartUnsupported = "starting Pandora at login is not supported on this system"

type autostartState struct {
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
}

type autostartRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	infos, err := s.core.Webhooks(r.Context())
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if infos == nil {
		infos = []core.WebhookInfo{}
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) handleRemoveWebhook(w http.ResponseWriter, r *http.Request) {
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	if _, err := s.registry.RemoveWebhook(ref); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if s.reloadWebhooks != nil {
		s.reloadWebhooks(r.Context())
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = value
	}
	items, err := s.core.RecentEvents(r.Context(), events.Filter{
		Service: r.URL.Query().Get("service"),
		Account: r.URL.Query().Get("account"),
		Topic:   r.URL.Query().Get("topic"),
		Limit:   limit,
	})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if items == nil {
		items = []events.Event{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	info, err := s.core.TunnelStatus(r.Context())
	if err != nil {
		info = core.TunnelInfo{Exposed: false, Problem: err.Error()}
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleAutostartStatus(w http.ResponseWriter, r *http.Request) {
	control, err := s.autostartControl()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if control == nil {
		writeJSON(w, http.StatusOK, autostartState{Supported: false, Enabled: false})
		return
	}
	enabled, err := control.Enabled(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, autostartState{Supported: true, Enabled: enabled})
}

func (s *Server) handleAutostartChange(w http.ResponseWriter, r *http.Request) {
	if !s.autostartAvailable() {
		writeError(w, http.StatusBadRequest, autostartUnsupported)
		return
	}
	var request autostartRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	control, err := s.autostartControl()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if control == nil {
		writeError(w, http.StatusBadRequest, autostartUnsupported)
		return
	}
	if request.Enabled {
		err = control.Enable(r.Context())
	} else {
		err = control.Disable(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, autostartState{Supported: true, Enabled: request.Enabled})
}

func (s *Server) autostartAvailable() bool {
	return (runtime.GOOS == "darwin" || runtime.GOOS == "linux") && s.autostart != nil
}

func (s *Server) autostartControl() (AutostartControl, error) {
	if !s.autostartAvailable() {
		return nil, nil
	}
	control, err := s.autostart()
	if err != nil {
		return nil, err
	}
	if control == nil {
		return nil, nil
	}
	return control, nil
}
