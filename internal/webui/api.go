package webui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/source"
)

type addAccountRequest struct {
	Service            string            `json:"service"`
	Account            string            `json:"account"`
	Kind               config.Kind       `json:"kind"`
	URL                string            `json:"url"`
	Command            []string          `json:"command"`
	Spec               string            `json:"spec"`
	Auth               config.AuthMode   `json:"auth"`
	OAuthClientID      string            `json:"oauth_client_id"`
	OAuthClientSecret  string            `json:"oauth_client_secret"`
	Scopes             []string          `json:"scopes"`
	ServiceDescription string            `json:"service_description"`
	Description        string            `json:"description"`
	Env                map[string]string `json:"env"`
	SecretEnv          map[string]string `json:"secret_env"`
	SecretHeaders      map[string]string `json:"secret_headers"`
}

type nameRequest struct {
	Name string `json:"name"`
}

type descriptionRequest struct {
	Ref  string `json:"ref"`
	Text string `json:"text"`
}

type callRequest struct {
	Ref  string         `json:"ref"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

type policyResponse struct {
	ReadOnly bool     `json:"readonly"`
	Allow    []string `json:"allow"`
	Deny     []string `json:"deny"`
}

type policyRequest struct {
	ReadOnly bool     `json:"readonly"`
	Allow    []string `json:"allow"`
	Deny     []string `json:"deny"`
}

type commandToolRequest struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	Command     []string                       `json:"command"`
	Params      map[string]config.CommandParam `json:"params"`
	ReadOnly    bool                           `json:"readonly"`
	TimeoutSec  int                            `json:"timeout_sec"`
}

type installRequest struct {
	MCP bool `json:"mcp"`
}

type catalogResponse struct {
	*core.Catalog
	Warning string `json:"warning,omitempty"`
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/overview", s.handleOverview)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/accounts", s.handleAddAccount)
	mux.HandleFunc("DELETE /api/accounts/{service}/{account}", s.handleRemoveAccount)
	mux.HandleFunc("DELETE /api/services/{service}", s.handleRemoveService)
	mux.HandleFunc("POST /api/accounts/{service}/{account}/rename", s.handleRenameAccount)
	mux.HandleFunc("POST /api/services/{service}/rename", s.handleRenameService)
	mux.HandleFunc("POST /api/accounts/{service}/{account}/default", s.handleDefault)
	mux.HandleFunc("PUT /api/descriptions", s.handleDescription)
	mux.HandleFunc("POST /api/accounts/{service}/{account}/login", s.handleLogin)
	mux.HandleFunc("GET /api/logins/{id}", s.handleLoginStatus)
	mux.HandleFunc("POST /api/accounts/{service}/{account}/logout", s.handleLogout)
	mux.HandleFunc("GET /api/services/{service}/tools", s.handleTools)
	mux.HandleFunc("POST /api/call", s.handleCall)
	mux.HandleFunc("GET /api/accounts/{service}/{account}/policy", s.handleGetPolicy)
	mux.HandleFunc("PUT /api/accounts/{service}/{account}/policy", s.handlePutPolicy)
	mux.HandleFunc("GET /api/log", s.handleLog)
	mux.HandleFunc("GET /api/doctor", s.handleDoctor)
	mux.HandleFunc("GET /api/integrations", s.handleIntegrations)
	mux.HandleFunc("POST /api/integrations/{client}/install", s.handleInstall)
	mux.HandleFunc("DELETE /api/integrations/{client}", s.handleUninstall)
	mux.HandleFunc("POST /api/services/{service}/command-tools", s.handleAddCommandTool)
	mux.HandleFunc("DELETE /api/services/{service}/command-tools/{name}", s.handleRemoveCommandTool)
	mux.HandleFunc("/api/", notFoundJSON)
	mux.HandleFunc("/api", notFoundJSON)
	mux.Handle("/", s.staticHandler())
	return mux
}

func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.core.Integrations(r.Context()))
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.core.Overview(r.Context())
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		writeJSON(w, http.StatusOK, s.core.StatusAll(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, []core.StatusInfo{s.core.Status(r.Context(), ref)})
}

func (s *Server) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	var request addAccountRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	err := s.registry.Add(registry.AddInput{
		Service:            request.Service,
		Account:            request.Account,
		Kind:               request.Kind,
		URL:                request.URL,
		Command:            request.Command,
		Spec:               request.Spec,
		Auth:               request.Auth,
		OAuthClientID:      request.OAuthClientID,
		OAuthClientSecret:  request.OAuthClientSecret,
		OAuthScopes:        request.Scopes,
		Description:        request.Description,
		ServiceDescription: request.ServiceDescription,
		Env:                request.Env,
		SecretEnv:          request.SecretEnv,
		SecretHeaders:      request.SecretHeaders,
	})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	ref := request.Service + "/" + request.Account
	status, detectErr := s.core.DetectAuth(r.Context(), ref)
	warning := ""
	if detectErr != nil {
		warning = detectErr.Error()
	}
	warning = joinWarning(warning, s.syncWarning(r.Context()))
	response := map[string]any{"ref": ref, "status": status}
	if warning != "" {
		response["warning"] = warning
	}
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handleRemoveAccount(w http.ResponseWriter, r *http.Request) {
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	if err := s.registry.RemoveAccount(ref); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleRemoveService(w http.ResponseWriter, r *http.Request) {
	if err := s.registry.RemoveService(r.PathValue("service")); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleRenameAccount(w http.ResponseWriter, r *http.Request) {
	var request nameRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	if err := s.registry.RenameAccount(ref, request.Name); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleRenameService(w http.ResponseWriter, r *http.Request) {
	var request nameRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := s.registry.RenameService(r.PathValue("service"), request.Name); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleDefault(w http.ResponseWriter, r *http.Request) {
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	if err := s.registry.SetDefault(ref); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleDescription(w http.ResponseWriter, r *http.Request) {
	var request descriptionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	ref, err := registry.ParseRef(request.Ref)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.registry.SetDescription(ref, request.Text); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	if _, err := s.registry.GetPolicy(ref); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	id := s.startLogin(ref.String())
	writeJSON(w, http.StatusAccepted, map[string]string{"login_id": id})
}

func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	state, ok := s.loginStatus(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "login not found")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	if err := s.core.Logout(r.Context(), ref.String()); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "1"
	catalog, err := s.core.Tools(r.Context(), r.PathValue("service"), refresh)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if refresh {
		if warning := s.syncWarning(r.Context()); warning != "" {
			writeJSON(w, http.StatusOK, catalogResponse{Catalog: catalog, Warning: warning})
			return
		}
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	var request callRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.core.Call(core.WithClient(r.Context(), "pandora-ui"), request.Ref, request.Tool, request.Args)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	account, err := s.registry.GetPolicy(ref)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, policyResponse{
		ReadOnly: account.ReadOnly,
		Allow:    sortedRules(account.Allow),
		Deny:     sortedRules(account.Deny),
	})
}

func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	var request policyRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	ref := registry.Ref{Service: r.PathValue("service"), Account: r.PathValue("account")}
	readOnly := request.ReadOnly
	account, err := s.registry.SetPolicy(ref, registry.PolicyInput{
		ReadOnly: &readOnly,
		Allow:    request.Allow,
		Deny:     request.Deny,
		Clear:    true,
	})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{
		"readonly": account.ReadOnly,
		"allow":    sortedRules(account.Allow),
		"deny":     sortedRules(account.Deny),
	}, s.syncWarning(r.Context()))
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("n"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "n must be zero or positive")
			return
		}
		limit = value
	}
	if s.callLog == nil {
		writeError(w, http.StatusInternalServerError, "call log is not configured")
		return
	}
	entries, err := s.callLog.Tail(limit, guard.Filter{
		Ref:        r.URL.Query().Get("ref"),
		OnlyErrors: r.URL.Query().Get("errors") == "1",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	offline := r.URL.Query().Get("offline") == "1"
	writeJSON(w, http.StatusOK, s.core.Doctor(r.Context(), offline))
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	client, err := install.ParseClient(r.PathValue("client"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request := installRequest{}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	reports, err := s.core.Install(r.Context(), []install.Client{client}, core.InstallOptions{MCP: request.MCP})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if reports == nil {
		reports = []core.InstallReport{}
	}
	writeJSON(w, http.StatusOK, reports)
}

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	client, err := install.ParseClient(r.PathValue("client"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reports, err := s.core.Uninstall(r.Context(), []install.Client{client})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if reports == nil {
		reports = []core.InstallReport{}
	}
	writeJSON(w, http.StatusOK, reports)
}

func (s *Server) handleAddCommandTool(w http.ResponseWriter, r *http.Request) {
	var request commandToolRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	err := s.registry.AddCommandTool(registry.CommandToolInput{
		Service:     r.PathValue("service"),
		Name:        request.Name,
		Description: request.Description,
		Command:     request.Command,
		Params:      request.Params,
		ReadOnly:    request.ReadOnly,
		TimeoutSec:  request.TimeoutSec,
	})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusCreated, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) handleRemoveCommandTool(w http.ResponseWriter, r *http.Request) {
	if err := s.registry.RemoveCommandTool(r.PathValue("service"), r.PathValue("name")); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeChange(w, http.StatusOK, map[string]any{"ok": true}, s.syncWarning(r.Context()))
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if info, err := fs.Stat(s.static, path); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		s.serveIndex(w)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	data, err := fs.ReadFile(s.static, "index.html")
	if err != nil {
		http.Error(w, "Pandora UI is not built yet. Run the UI build.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) syncWarning(ctx context.Context) string {
	if _, err := s.core.SyncSkills(ctx); err != nil {
		return err.Error()
	}
	return ""
}

func writeChange(w http.ResponseWriter, status int, value map[string]any, warning string) {
	if warning != "" {
		value["warning"] = warning
	}
	writeJSON(w, status, value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func notFoundJSON(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func sortedRules(rules []string) []string {
	out := append([]string(nil), rules...)
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}

func joinWarning(existing, added string) string {
	switch {
	case existing == "":
		return added
	case added == "":
		return existing
	default:
		return existing + "; " + added
	}
}

func statusForError(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	var authErr *source.AuthRequiredError
	if errors.As(err, &authErr) {
		return http.StatusBadRequest
	}
	var blocked *core.BlockedError
	if errors.As(err, &blocked) {
		return http.StatusForbidden
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "unknown tool"):
		return http.StatusBadRequest
	case strings.Contains(text, "unknown service"),
		strings.Contains(text, "unknown account"),
		strings.Contains(text, "not found"):
		return http.StatusNotFound
	case strings.Contains(text, "invalid"),
		strings.Contains(text, "already exists"),
		strings.Contains(text, "does not"),
		strings.Contains(text, "not supported"),
		strings.Contains(text, "cannot"),
		strings.Contains(text, "needs"),
		strings.Contains(text, "is required"),
		strings.Contains(text, "has no default account"),
		strings.Contains(text, "is not a command service"),
		strings.Contains(text, "unsupported"),
		strings.Contains(text, "no ai clients"):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
