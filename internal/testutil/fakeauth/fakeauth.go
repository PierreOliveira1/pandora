package fakeauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type Server struct {
	URL string

	accessTTL           time.Duration
	disableRegistration bool
	staticClientID      string

	mu            sync.Mutex
	clients       map[string]*registeredClient
	codes         map[string]*authorizationCode
	accessTokens  map[string]*accessToken
	refreshTokens map[string]*refreshToken
	nextClient    int

	registrationCount atomic.Int64
	refreshCount      atomic.Int64
}

type Options struct {
	DisableRegistration bool
	StaticClientID      string
	AccessTTL           time.Duration
}

type registeredClient struct {
	redirectURIs []string
}

type authorizationCode struct {
	clientID    string
	redirectURI string
	challenge   string
	resource    string
	scope       string
	account     string
	expires     time.Time
}

type accessToken struct {
	clientID string
	account  string
	scope    string
	resource string
	expires  time.Time
}

type refreshToken struct {
	clientID string
	account  string
	scope    string
	resource string
}

func New(t testing.TB, o Options) *Server {
	t.Helper()
	ttl := o.AccessTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	s := &Server{
		accessTTL:           ttl,
		disableRegistration: o.DisableRegistration,
		staticClientID:      o.StaticClientID,
		clients:             map[string]*registeredClient{},
		codes:               map[string]*authorizationCode{},
		accessTokens:        map[string]*accessToken{},
		refreshTokens:       map[string]*refreshToken{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", s.handleResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource", s.handleResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.handleAuthorizationServerMetadata)
	mux.HandleFunc("/.well-known/openid-configuration", s.handleAuthorizationServerMetadata)
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/authorize", s.handleAuthorize)
	mux.HandleFunc("/token", s.handleToken)
	server := httptest.NewServer(mux)
	s.URL = server.URL
	t.Cleanup(server.Close)
	return s
}

func (s *Server) MCPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := s.accountForToken(bearerToken(r.Header.Get("Authorization")))
		if !ok {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp", scope="tasks"`, s.URL))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-Account", account)
		if next == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) RefreshCount() int {
	return int(s.refreshCount.Load())
}

func (s *Server) RegistrationCount() int {
	return int(s.registrationCount.Load())
}

func (s *Server) handleResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":              s.URL + "/mcp",
		"authorization_servers": []string{s.URL},
		"scopes_supported":      []string{"tasks"},
	})
}

func (s *Server) handleAuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	metadata := map[string]any{
		"issuer":                           s.URL,
		"authorization_endpoint":           s.URL + "/authorize",
		"token_endpoint":                   s.URL + "/token",
		"code_challenge_methods_supported": []string{"S256"},
		"scopes_supported":                 []string{"tasks"},
	}
	if !s.disableRegistration {
		metadata["registration_endpoint"] = s.URL + "/register"
	}
	writeJSON(w, http.StatusOK, metadata)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if s.disableRegistration {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
		return
	}
	s.mu.Lock()
	s.nextClient++
	clientID := fmt.Sprintf("client-%d", s.nextClient)
	s.clients[clientID] = &registeredClient{redirectURIs: append([]string(nil), request.RedirectURIs...)}
	s.mu.Unlock()
	s.registrationCount.Add(1)
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"redirect_uris":              request.RedirectURIs,
		"token_endpoint_auth_method": "none",
	})
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	clientID := query.Get("client_id")
	if !s.clientKnown(clientID) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client")
		return
	}
	redirectURI := query.Get("redirect_uri")
	if !s.redirectAllowed(clientID, redirectURI) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is not registered")
		return
	}
	if query.Get("response_type") != "code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_response_type", "response_type must be code")
		return
	}
	challenge := query.Get("code_challenge")
	if challenge == "" || query.Get("code_challenge_method") != "S256" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "PKCE S256 is required")
		return
	}
	resource := query.Get("resource")
	if resource == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "resource is required")
		return
	}
	scope := query.Get("scope")
	if scope == "" {
		scope = "tasks"
	}
	account := query.Get("login_hint")
	if account == "" {
		account = "user"
	}
	code := randomToken()
	s.mu.Lock()
	s.codes[code] = &authorizationCode{
		clientID:    clientID,
		redirectURI: redirectURI,
		challenge:   challenge,
		resource:    resource,
		scope:       scope,
		account:     account,
		expires:     time.Now().Add(5 * time.Minute),
	}
	s.mu.Unlock()

	destination, err := url.Parse(redirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is invalid")
		return
	}
	destinationQuery := destination.Query()
	destinationQuery.Set("code", code)
	if state := query.Get("state"); state != "" {
		destinationQuery.Set("state", state)
	}
	destination.RawQuery = destinationQuery.Encode()
	http.Redirect(w, r, destination.String(), http.StatusFound)
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid form")
		return
	}
	clientID, _ := tokenClientID(r)
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		s.exchangeCode(w, r, clientID)
	case "refresh_token":
		s.exchangeRefreshToken(w, r, clientID)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant_type")
	}
}

func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request, clientID string) {
	code := r.Form.Get("code")
	s.mu.Lock()
	stored, ok := s.codes[code]
	if ok {
		delete(s.codes, code)
	}
	s.mu.Unlock()
	if !ok || time.Now().After(stored.expires) || clientID != stored.clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid")
		return
	}
	if r.Form.Get("redirect_uri") != stored.redirectURI || r.Form.Get("resource") != stored.resource {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri or resource does not match")
		return
	}
	verifier := r.Form.Get("code_verifier")
	computed := oauth2.S256ChallengeFromVerifier(verifier)
	if verifier == "" || subtle.ConstantTimeCompare([]byte(computed), []byte(stored.challenge)) != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	writeJSON(w, http.StatusOK, s.issueTokens(stored.clientID, stored.account, stored.scope, stored.resource))
}

func (s *Server) exchangeRefreshToken(w http.ResponseWriter, r *http.Request, clientID string) {
	value := r.Form.Get("refresh_token")
	s.mu.Lock()
	stored, ok := s.refreshTokens[value]
	if ok && (clientID == "" || clientID == stored.clientID) {
		delete(s.refreshTokens, value)
	} else {
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid")
		return
	}
	s.refreshCount.Add(1)
	writeJSON(w, http.StatusOK, s.issueTokens(stored.clientID, stored.account, stored.scope, stored.resource))
}

func (s *Server) issueTokens(clientID, account, scope, resource string) map[string]any {
	access := randomToken()
	refresh := randomToken()
	s.mu.Lock()
	s.accessTokens[access] = &accessToken{
		clientID: clientID,
		account:  account,
		scope:    scope,
		resource: resource,
		expires:  time.Now().Add(s.accessTTL),
	}
	s.refreshTokens[refresh] = &refreshToken{
		clientID: clientID,
		account:  account,
		scope:    scope,
		resource: resource,
	}
	s.mu.Unlock()
	seconds := int64(math.Ceil(s.accessTTL.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    seconds,
		"refresh_token": refresh,
		"scope":         scope,
	}
}

func (s *Server) clientKnown(clientID string) bool {
	if clientID != "" && clientID == s.staticClientID {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.clients[clientID]
	return ok
}

func (s *Server) redirectAllowed(clientID, redirectURI string) bool {
	if clientID != "" && clientID == s.staticClientID {
		return strings.HasPrefix(redirectURI, "http://localhost:") || strings.HasPrefix(redirectURI, "http://127.0.0.1:")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client, ok := s.clients[clientID]
	if !ok {
		return false
	}
	for _, registered := range client.redirectURIs {
		if registered == redirectURI {
			return true
		}
	}
	return false
}

func (s *Server) accountForToken(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.accessTokens[token]
	if !ok || time.Now().After(stored.expires) {
		return "", false
	}
	return stored.account, true
}

func tokenClientID(r *http.Request) (string, string) {
	if clientID := r.Form.Get("client_id"); clientID != "" {
		return clientID, r.Form.Get("client_secret")
	}
	if clientID, secret, ok := r.BasicAuth(); ok {
		return clientID, secret
	}
	return "", ""
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func randomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("fakeauth: generating token: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(value[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{
		"error":             code,
		"error_description": description,
	})
}
