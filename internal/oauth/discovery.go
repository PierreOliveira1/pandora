package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Metadata struct {
	Resource              string
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	RegistrationEndpoint  string
	Scopes                []string
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

type authorizationServerMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	ScopesSupported               []string `json:"scopes_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"pandora","version":"0.0.0"}}}`

func Discover(ctx context.Context, hc *http.Client, mcpURL string) (*Metadata, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	mcp, err := url.Parse(mcpURL)
	if err != nil {
		return nil, fmt.Errorf("invalid MCP URL %q: %w", mcpURL, err)
	}
	if mcp.Scheme == "" || mcp.Host == "" {
		return nil, fmt.Errorf("invalid MCP URL %q: it must be absolute", mcpURL)
	}

	params := map[string]string{}
	if resp, err := initializeRequestTo(ctx, hc, mcpURL); err == nil {
		params = authenticateParams(resp.Header.Values("WWW-Authenticate"))
		_ = resp.Body.Close()
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	headerScopes := strings.Fields(params["scope"])

	resource, resourceURL := fetchResourceMetadata(ctx, hc, mcp, params["resource_metadata"])
	issuer := mcp.Scheme + "://" + mcp.Host
	resourceValue := mcpURL
	scopes := headerScopes
	if resource != nil {
		if len(resource.AuthorizationServers) == 0 {
			return nil, fmt.Errorf("resource metadata at %s does not list authorization_servers; check the server OAuth configuration", resourceURL)
		}
		issuer = resource.AuthorizationServers[0]
		if resource.Resource != "" {
			resourceValue = resource.Resource
		}
		if len(scopes) == 0 {
			scopes = resource.ScopesSupported
		}
	}

	server, metadataURL, err := fetchAuthorizationServerMetadata(ctx, hc, issuer)
	if err != nil {
		return nil, err
	}
	if len(server.CodeChallengeMethodsSupported) > 0 && !containsString(server.CodeChallengeMethodsSupported, "S256") {
		return nil, fmt.Errorf("authorization server %s does not support PKCE S256; Pandora requires PKCE S256", issuer)
	}
	var missing []string
	if server.AuthorizationEndpoint == "" {
		missing = append(missing, "authorization_endpoint")
	}
	if server.TokenEndpoint == "" {
		missing = append(missing, "token_endpoint")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("OAuth metadata at %s is missing %s; check the authorization server metadata", metadataURL, strings.Join(missing, " and "))
	}
	return &Metadata{
		Resource:              resourceValue,
		Issuer:                issuer,
		AuthorizationEndpoint: server.AuthorizationEndpoint,
		TokenEndpoint:         server.TokenEndpoint,
		RegistrationEndpoint:  server.RegistrationEndpoint,
		Scopes:                scopes,
	}, nil
}

func initializeRequestTo(ctx context.Context, hc *http.Client, mcpURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, strings.NewReader(initializeRequest))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return hc.Do(req)
}

func fetchResourceMetadata(ctx context.Context, hc *http.Client, mcp *url.URL, headerURL string) (*protectedResourceMetadata, string) {
	for _, candidate := range resourceMetadataURLs(mcp, headerURL) {
		var resource protectedResourceMetadata
		found, err := fetchJSON(ctx, hc, candidate, &resource)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ""
		}
		if found {
			return &resource, candidate
		}
	}
	return nil, ""
}

func resourceMetadataURLs(mcp *url.URL, headerURL string) []string {
	var urls []string
	add := func(value string) {
		if value == "" {
			return
		}
		for _, existing := range urls {
			if existing == value {
				return
			}
		}
		urls = append(urls, value)
	}
	add(headerURL)
	origin := (&url.URL{Scheme: mcp.Scheme, Host: mcp.Host}).String()
	path := strings.TrimSuffix(mcp.EscapedPath(), "/")
	if path != "" && path != "/" {
		add(origin + "/.well-known/oauth-protected-resource" + path)
	}
	add(origin + "/.well-known/oauth-protected-resource")
	return urls
}

func fetchAuthorizationServerMetadata(ctx context.Context, hc *http.Client, issuer string) (*authorizationServerMetadata, string, error) {
	candidates, err := authorizationServerMetadataURLs(issuer)
	if err != nil {
		return nil, "", err
	}
	var lastErr error
	for _, candidate := range candidates {
		var server authorizationServerMetadata
		found, err := fetchJSON(ctx, hc, candidate, &server)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, "", err
			}
			lastErr = err
			continue
		}
		if found {
			return &server, candidate, nil
		}
	}
	if lastErr != nil {
		return nil, "", fmt.Errorf("loading OAuth metadata for %s: %w", issuer, lastErr)
	}
	return nil, "", fmt.Errorf("no OAuth metadata found for %s; check the authorization server discovery endpoints", issuer)
}

func authorizationServerMetadataURLs(issuer string) ([]string, error) {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return nil, fmt.Errorf("invalid authorization server URL %q: %w", issuer, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid authorization server URL %q: it must be absolute", issuer)
	}
	origin := &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
	path := strings.TrimSuffix(parsed.Path, "/")
	if path == "" || path == "/" {
		return []string{
			withPath(origin, "/.well-known/oauth-authorization-server"),
			withPath(origin, "/.well-known/openid-configuration"),
		}, nil
	}
	return []string{
		withPath(origin, "/.well-known/oauth-authorization-server"+path),
		withPath(origin, "/.well-known/openid-configuration"+path),
		withPath(origin, path+"/.well-known/openid-configuration"),
	}, nil
}

func withPath(base *url.URL, path string) string {
	copied := *base
	copied.Path = path
	return copied.String()
}

func fetchJSON(ctx context.Context, hc *http.Client, endpoint string, target any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return false, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, nil
	}
	return true, nil
}

func authenticateParams(headers []string) map[string]string {
	params := map[string]string{}
	for _, header := range headers {
		scanAuthenticateParams(header, params)
	}
	return params
}

func scanAuthenticateParams(header string, params map[string]string) {
	for i := 0; i < len(header); {
		for i < len(header) && (header[i] == ' ' || header[i] == '\t' || header[i] == ',') {
			i++
		}
		start := i
		for i < len(header) && isParamNameByte(header[i]) {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := strings.ToLower(header[start:i])
		j := i
		for j < len(header) && (header[j] == ' ' || header[j] == '\t') {
			j++
		}
		if j >= len(header) || header[j] != '=' {
			continue
		}
		i = j + 1
		for i < len(header) && (header[i] == ' ' || header[i] == '\t') {
			i++
		}
		value, next := readParamValue(header, i)
		params[name] = value
		i = next
	}
}

func readParamValue(header string, start int) (string, int) {
	if start >= len(header) {
		return "", start
	}
	if header[start] == '"' {
		var b strings.Builder
		for i := start + 1; i < len(header); i++ {
			if header[i] == '\\' && i+1 < len(header) {
				b.WriteByte(header[i+1])
				i++
				continue
			}
			if header[i] == '"' {
				return b.String(), i + 1
			}
			b.WriteByte(header[i])
		}
		return b.String(), len(header)
	}
	i := start
	for i < len(header) && header[i] != ',' && header[i] != ';' && header[i] != ' ' && header[i] != '\t' {
		i++
	}
	return header[start:i], i
}

func isParamNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.'
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
