package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/PierreOliveira1/pandora/internal/secrets"
)

type Client struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	RedirectURI  string `json:"redirect_uri"`
	Issuer       string `json:"issuer"`
}

type registrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type registrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

func (m *Manager) resolveClient(ctx context.Context, service, redirect string, md *Metadata, clientID string) (Client, error) {
	if clientID != "" {
		client := Client{ClientID: clientID, RedirectURI: redirect, Issuer: md.Issuer}
		saved, err := m.savedClient(service)
		if err != nil {
			return Client{}, err
		}
		if saved != nil && saved.ClientID == clientID {
			client.ClientSecret = saved.ClientSecret
		}
		return client, nil
	}
	saved, err := m.savedClient(service)
	if err != nil {
		return Client{}, err
	}
	if saved != nil && saved.Issuer == md.Issuer && saved.RedirectURI == redirect {
		return *saved, nil
	}
	if md.RegistrationEndpoint == "" {
		return Client{}, fmt.Errorf("%s does not support automatic client registration: create an OAuth app with redirect URL %s and run pandora add %s ... --client-id <id> [--client-secret]", service, redirect, service)
	}
	return m.registerClient(ctx, service, redirect, md)
}

func (m *Manager) savedClient(service string) (*Client, error) {
	value, err := m.secrets.Get(secrets.OAuthClientKey(service))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading saved OAuth client for %s: %w", service, err)
	}
	var client Client
	if err := json.Unmarshal([]byte(value), &client); err != nil {
		return nil, fmt.Errorf("decoding saved OAuth client for %s: %w", service, err)
	}
	return &client, nil
}

func (m *Manager) registerClient(ctx context.Context, service, redirect string, md *Metadata) (Client, error) {
	body, err := json.Marshal(registrationRequest{
		ClientName:              "Pandora",
		RedirectURIs:            []string{redirect},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
	})
	if err != nil {
		return Client{}, fmt.Errorf("encoding OAuth registration for %s: %w", service, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, md.RegistrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return Client{}, fmt.Errorf("creating OAuth registration request for %s: %w", service, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := m.hc.Do(req)
	if err != nil {
		return Client{}, fmt.Errorf("registering OAuth client for %s: %w", service, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Client{}, fmt.Errorf("registering OAuth client for %s: registration endpoint returned %s", service, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Client{}, fmt.Errorf("reading OAuth registration response for %s: %w", service, err)
	}
	var registered registrationResponse
	if err := json.Unmarshal(data, &registered); err != nil {
		return Client{}, fmt.Errorf("decoding OAuth registration response for %s: %w", service, err)
	}
	if registered.ClientID == "" {
		return Client{}, fmt.Errorf("registering OAuth client for %s: registration endpoint did not return a client_id", service)
	}
	client := Client{
		ClientID:     registered.ClientID,
		ClientSecret: registered.ClientSecret,
		RedirectURI:  redirect,
		Issuer:       md.Issuer,
	}
	saved, err := json.Marshal(client)
	if err != nil {
		return Client{}, fmt.Errorf("encoding OAuth client for %s: %w", service, err)
	}
	if err := m.secrets.Set(secrets.OAuthClientKey(service), string(saved)); err != nil {
		return Client{}, fmt.Errorf("saving OAuth client for %s: %w", service, err)
	}
	return client, nil
}
