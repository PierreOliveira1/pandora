package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/secrets"
)

type Manager struct {
	hc          *http.Client
	secrets     secrets.Store
	paths       config.Paths
	now         func() time.Time
	openBrowser func(url string) error
	out         io.Writer
}

type Options struct {
	HTTPClient  *http.Client
	Secrets     secrets.Store
	Paths       config.Paths
	OpenBrowser func(url string) error
	Out         io.Writer
	Now         func() time.Time
}

type LoginParams struct {
	Service      string
	Account      string
	MCPURL       string
	ClientID     string
	Scopes       []string
	RedirectPort int
}

type callbackResult struct {
	code string
	err  error
}

const (
	defaultRedirectPort = 33418
	loginTimeout        = 5 * time.Minute
	loginSuccessHTML    = `Pandora: login complete. You can close this tab.`
)

func NewManager(o Options) *Manager {
	hc := o.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	return &Manager{
		hc:          hc,
		secrets:     o.Secrets,
		paths:       o.Paths,
		now:         now,
		openBrowser: o.OpenBrowser,
		out:         out,
	}
}

func (m *Manager) Login(ctx context.Context, p LoginParams) error {
	md, err := Discover(ctx, m.hc, p.MCPURL)
	if err != nil {
		return fmt.Errorf("discovering OAuth settings for %s: %w", p.Service, err)
	}
	port := p.RedirectPort
	if port == 0 {
		port = defaultRedirectPort
	}
	redirect := fmt.Sprintf("http://localhost:%d/callback", port)
	client, err := m.resolveClient(ctx, p.Service, redirect, md, p.ClientID)
	if err != nil {
		return err
	}
	scopes := p.Scopes
	if len(scopes) == 0 {
		scopes = md.Scopes
	}
	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return err
	}
	oauthConfig := &oauth2.Config{
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  md.AuthorizationEndpoint,
			TokenURL: md.TokenEndpoint,
		},
		RedirectURL: redirect,
		Scopes:      scopes,
	}
	loginURL := oauthConfig.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", md.Resource))

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("port %d is busy: set oauth.redirect_port for this service", port)
	}
	defer listener.Close()

	listenerV6, errV6 := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(port)))
	if errV6 == nil {
		defer listenerV6.Close()
	}

	results := make(chan callbackResult, 1)
	var deliver sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		var result callbackResult
		switch {
		case query.Get("state") != state:
			result.err = fmt.Errorf("OAuth state mismatch: the login response did not match this login")
			http.Error(w, "Pandora: login failed: invalid state.", http.StatusBadRequest)
		case query.Get("error") != "":
			code := query.Get("error")
			if description := query.Get("error_description"); description != "" {
				result.err = fmt.Errorf("authorization server returned %s: %s", code, description)
			} else {
				result.err = fmt.Errorf("authorization server returned %s", code)
			}
			http.Error(w, "Pandora: login failed. You can close this tab.", http.StatusBadRequest)
		case query.Get("code") == "":
			result.err = fmt.Errorf("OAuth callback did not include a code")
			http.Error(w, "Pandora: login failed: missing code.", http.StatusBadRequest)
		default:
			result.code = query.Get("code")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, loginSuccessHTML)
		}
		deliver.Do(func() {
			results <- result
		})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		_ = server.Serve(listener)
	}()
	if listenerV6 != nil {
		go func() {
			_ = server.Serve(listenerV6)
		}()
	}
	defer server.Close()

	_, _ = fmt.Fprintf(m.out, "Open this URL to log in to %s/%s: %s\n", p.Service, p.Account, loginURL)
	if m.openBrowser != nil {
		_ = m.openBrowser(loginURL)
	}

	timer := time.NewTimer(loginTimeout)
	defer timer.Stop()
	var result callbackResult
	select {
	case result = <-results:
	case <-timer.C:
		return fmt.Errorf("timed out waiting for the OAuth callback; open the login URL and try again")
	case <-ctx.Done():
		return ctx.Err()
	}
	if result.err != nil {
		return result.err
	}
	exchangeCtx := context.WithValue(ctx, oauth2.HTTPClient, m.hc)
	token, err := oauthConfig.Exchange(exchangeCtx, result.code, oauth2.VerifierOption(verifier), oauth2.SetAuthURLParam("resource", md.Resource))
	if err != nil {
		return fmt.Errorf("exchanging the OAuth code for %s/%s: %w", p.Service, p.Account, err)
	}
	record := &Record{
		Token:        token,
		TokenURL:     md.TokenEndpoint,
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		Resource:     md.Resource,
		Scopes:       scopes,
	}
	return m.writeRecord(p.Service, p.Account, record)
}

func randomState() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generating OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
