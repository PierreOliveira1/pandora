package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/oauth2"

	"github.com/PierreOliveira1/pandora/internal/secrets"
)

type Record struct {
	Token        *oauth2.Token `json:"token"`
	TokenURL     string        `json:"token_url"`
	ClientID     string        `json:"client_id"`
	ClientSecret string        `json:"client_secret,omitempty"`
	Resource     string        `json:"resource"`
	Scopes       []string      `json:"scopes,omitempty"`
}

var ErrLoginRequired = errors.New("login required")

func (m *Manager) AccessToken(ctx context.Context, service, account string) (string, error) {
	record, err := m.readRecord(service, account)
	if err != nil {
		return "", err
	}
	if m.tokenValid(record.Token) {
		return record.Token.AccessToken, nil
	}
	return m.refresh(ctx, service, account, record, false)
}

func (m *Manager) ForceRefresh(ctx context.Context, service, account string) (string, error) {
	record, err := m.readRecord(service, account)
	if err != nil {
		return "", err
	}
	return m.refresh(ctx, service, account, record, true)
}

func (m *Manager) Logout(service, account string) error {
	err := m.secrets.Delete(secrets.OAuthTokenKey(service, account))
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return fmt.Errorf("removing OAuth token for %s/%s: %w", service, account, err)
	}
	return nil
}

func (m *Manager) HasToken(service, account string) bool {
	value, err := m.secrets.Get(secrets.OAuthTokenKey(service, account))
	if err != nil {
		return false
	}
	var record Record
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return false
	}
	return record.Token != nil && record.Token.AccessToken != ""
}

func (m *Manager) refresh(ctx context.Context, service, account string, initial *Record, force bool) (string, error) {
	lock, err := m.lockAccount(ctx, service, account)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = lock.Unlock()
		_ = lock.Close()
	}()

	current, err := m.readRecord(service, account)
	if err != nil {
		return "", err
	}
	if force {
		if !sameToken(initial.Token, current.Token) && current.Token != nil && current.Token.AccessToken != "" {
			return current.Token.AccessToken, nil
		}
	} else if m.tokenValid(current.Token) {
		return current.Token.AccessToken, nil
	}
	if current.Token == nil || current.Token.RefreshToken == "" {
		return "", ErrLoginRequired
	}
	if current.TokenURL == "" {
		return "", fmt.Errorf("saved OAuth token for %s/%s has no token URL; run pandora auth %s/%s again", service, account, service, account)
	}
	config := &oauth2.Config{
		ClientID:     current.ClientID,
		ClientSecret: current.ClientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: current.TokenURL},
		Scopes:       current.Scopes,
	}
	refreshToken := &oauth2.Token{RefreshToken: current.Token.RefreshToken}
	refreshCtx := context.WithValue(ctx, oauth2.HTTPClient, m.hc)
	refreshed, err := config.TokenSource(refreshCtx, refreshToken).Token()
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
			return "", ErrLoginRequired
		}
		return "", fmt.Errorf("refreshing OAuth token for %s/%s: %w", service, account, err)
	}
	if refreshed.AccessToken == "" {
		return "", fmt.Errorf("refreshing OAuth token for %s/%s: the token endpoint returned no access token", service, account)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = current.Token.RefreshToken
	}
	if refreshed.TokenType == "" {
		refreshed.TokenType = current.Token.TokenType
	}
	updated := &Record{
		Token:        refreshed,
		TokenURL:     current.TokenURL,
		ClientID:     current.ClientID,
		ClientSecret: current.ClientSecret,
		Resource:     current.Resource,
		Scopes:       current.Scopes,
	}
	if err := m.writeRecord(service, account, updated); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (m *Manager) readRecord(service, account string) (*Record, error) {
	value, err := m.secrets.Get(secrets.OAuthTokenKey(service, account))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, ErrLoginRequired
	}
	if err != nil {
		return nil, fmt.Errorf("reading OAuth token for %s/%s: %w", service, account, err)
	}
	var record Record
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return nil, fmt.Errorf("decoding OAuth token for %s/%s: %w", service, account, err)
	}
	if record.Token == nil {
		return nil, ErrLoginRequired
	}
	return &record, nil
}

func (m *Manager) writeRecord(service, account string, record *Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encoding OAuth token for %s/%s: %w", service, account, err)
	}
	if err := m.secrets.Set(secrets.OAuthTokenKey(service, account), string(data)); err != nil {
		return fmt.Errorf("saving OAuth token for %s/%s: %w", service, account, err)
	}
	return nil
}

func (m *Manager) tokenValid(token *oauth2.Token) bool {
	if token == nil || token.AccessToken == "" {
		return false
	}
	if token.Expiry.IsZero() {
		return true
	}
	now := time.Now
	if m.now != nil {
		now = m.now
	}
	return token.Expiry.Add(-60 * time.Second).After(now())
}

func (m *Manager) lockAccount(ctx context.Context, service, account string) (*flock.Flock, error) {
	path := m.paths.AccountLock(service, account)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating the account lock directory for %s/%s: %w", service, account, err)
	}
	lock := flock.New(path, flock.SetPermissions(0o600))
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(lockCtx, 50*time.Millisecond)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("waiting for the account lock for %s/%s: %w", service, account, err)
	}
	if !locked {
		_ = lock.Close()
		return nil, fmt.Errorf("waiting for the account lock for %s/%s: timed out", service, account)
	}
	return lock, nil
}

func sameToken(a, b *oauth2.Token) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.AccessToken == b.AccessToken && a.RefreshToken == b.RefreshToken
}
