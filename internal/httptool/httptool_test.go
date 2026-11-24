package httptool_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
)

func apiService() *config.Service {
	return &config.Service{URLs: map[string]string{"api": "https://api.example.com"}}
}

func TestAnArgumentInThePathDoesNotCreateSegments(t *testing.T) {
	svc := apiService()
	tool := config.CommandTool{
		Name: "item",
		HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/items/{{id}}"},
		Params: map[string]config.CommandParam{
			"id": {Type: "string"},
		},
	}
	request, err := httptool.Build(context.Background(), httptool.Input{
		Tool:    tool,
		Service: svc,
		Args:    map[string]any{"id": "a/b"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := request.URL.EscapedPath(); got != "/items/a%2Fb" {
		t.Errorf("escaped path = %q, want /items/a%%2Fb", got)
	}
}

func TestAnOptionalArgumentMissingFromAQueryIsOmitted(t *testing.T) {
	svc := apiService()
	tool := config.CommandTool{
		Name: "search",
		HTTP: &config.HTTPRequest{
			URL:    "api",
			Method: http.MethodGet,
			Path:   "/search",
			Query:  map[string]string{"q": "{{term}}", "page": "1"},
		},
		Params: map[string]config.CommandParam{
			"term": {Type: "string"},
		},
	}

	request, err := httptool.Build(context.Background(), httptool.Input{Tool: tool, Service: svc})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := request.URL.RawQuery; got != "page=1" {
		t.Errorf("query = %q, want page=1", got)
	}

	request, err = httptool.Build(context.Background(), httptool.Input{
		Tool:    tool,
		Service: svc,
		Args:    map[string]any{"term": "pandora"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := request.URL.RawQuery; got != "page=1&q=pandora" {
		t.Errorf("query = %q, want page=1&q=pandora", got)
	}
}

func TestAServiceHeaderUsesTheAccountSecret(t *testing.T) {
	svc := apiService()
	svc.Headers = map[string]string{"Authorization": "Bearer {{env.TOKEN}}"}
	tool := config.CommandTool{
		Name: "me",
		HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/me"},
	}
	request, err := httptool.Build(context.Background(), httptool.Input{
		Tool:      tool,
		Service:   svc,
		Env:       map[string]string{"TOKEN": "secret-value"},
		SecretEnv: map[string]bool{"TOKEN": true},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer secret-value" {
		t.Errorf("Authorization = %q, want the secret value", got)
	}
}

func TestASecretCannotBeUsedOutsideServiceHeaders(t *testing.T) {
	tests := []struct {
		name string
		tool config.CommandTool
	}{
		{
			name: "path",
			tool: config.CommandTool{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/{{env.TOKEN}}"}},
		},
		{
			name: "query",
			tool: config.CommandTool{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/", Query: map[string]string{"q": "{{env.TOKEN}}"}}},
		},
		{
			name: "form",
			tool: config.CommandTool{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodPost, Path: "/", Form: map[string]string{"q": "{{env.TOKEN}}"}}},
		},
		{
			name: "json",
			tool: config.CommandTool{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodPost, Path: "/", JSON: map[string]string{"q": "{{env.TOKEN}}"}}},
		},
		{
			name: "header",
			tool: config.CommandTool{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/", Headers: map[string]string{"X-Token": "{{env.TOKEN}}"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := httptool.Validate(tt.tool, apiService(), map[string]bool{"TOKEN": true})
			if err == nil || !strings.Contains(err.Error(), "the secret TOKEN can only be used in the service headers") {
				t.Fatalf("Validate() error = %v, want the secret restriction", err)
			}
		})
	}
}

func TestTheJSONBodyKeepsArgumentTypesAndNestsDottedKeys(t *testing.T) {
	svc := apiService()
	tool := config.CommandTool{
		Name: "create",
		HTTP: &config.HTTPRequest{
			URL:    "api",
			Method: http.MethodPost,
			Path:   "/users",
			JSON: map[string]string{
				"user.name": "{{name}}",
				"user.age":  "{{age}}",
				"active":    "{{active}}",
				"tags":      "{{tags}}",
				"label":     "user-{{name}}",
				"missing":   "{{absent}}",
			},
		},
		Params: map[string]config.CommandParam{
			"name":   {Type: "string"},
			"age":    {Type: "integer"},
			"active": {Type: "boolean"},
			"tags":   {Type: "array"},
			"absent": {Type: "string"},
		},
	}
	request, err := httptool.Build(context.Background(), httptool.Input{
		Tool:    tool,
		Service: svc,
		Args: map[string]any{
			"name":   "Ana",
			"age":    42,
			"active": true,
			"tags":   []string{"a", "b"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := request.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decoding body %q: %v", data, err)
	}
	user, ok := body["user"].(map[string]any)
	if !ok {
		t.Fatalf("user = %v, want an object", body["user"])
	}
	if user["name"] != "Ana" || user["age"] != float64(42) {
		t.Errorf("user = %v, want name Ana and a numeric age", user)
	}
	if body["active"] != true {
		t.Errorf("active = %v, want true", body["active"])
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %v, want the array", body["tags"])
	}
	if body["label"] != "user-Ana" {
		t.Errorf("label = %v, want user-Ana", body["label"])
	}
	if _, ok := body["missing"]; ok {
		t.Errorf("missing field was included: %v", body["missing"])
	}
}

func TestAToolMustChooseAURLWhenTheServiceHasSeveral(t *testing.T) {
	svc := &config.Service{URLs: map[string]string{
		"a": "https://a.example.com",
		"b": "https://b.example.com",
	}}
	tool := config.CommandTool{Name: "call", HTTP: &config.HTTPRequest{Method: http.MethodGet, Path: "/"}}
	err := httptool.Validate(tool, svc, nil)
	want := "the service has several URLs: choose one with --url"
	if err == nil || err.Error() != want {
		t.Fatalf("Validate() error = %v, want %q", err, want)
	}

	tool.HTTP.URL = "a"
	if err := httptool.Validate(tool, svc, nil); err != nil {
		t.Fatalf("Validate() with a URL error = %v", err)
	}

	err = httptool.Validate(tool, &config.Service{}, nil)
	want = "service has no URL: add one with pandora http url"
	if err == nil || err.Error() != want {
		t.Fatalf("Validate() without URLs error = %v, want %q", err, want)
	}
}

func TestGETWithABodyIsRefused(t *testing.T) {
	tests := []config.CommandTool{
		{Name: "get", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/", Form: map[string]string{"a": "b"}}},
		{Name: "head", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodHead, Path: "/", JSON: map[string]string{"a": "b"}}},
	}
	for _, tool := range tests {
		err := httptool.Validate(tool, apiService(), nil)
		if err == nil || !strings.Contains(err.Error(), "cannot have a form or JSON body") {
			t.Errorf("Validate(%s) error = %v, want a body error", tool.HTTP.Method, err)
		}
	}
}

func TestValidateURLOnlyAllowsHTTPOnLocalhost(t *testing.T) {
	valid := []string{
		"https://example.com",
		"https://example.com:8443/path",
		"http://localhost:8080",
		"http://127.0.0.1:1234",
		"http://[::1]:8080",
	}
	for _, raw := range valid {
		if err := httptool.ValidateURL(raw); err != nil {
			t.Errorf("ValidateURL(%q) error = %v, want nil", raw, err)
		}
	}

	invalid := map[string]string{
		"http://example.com":         "the URL must use https (http is allowed only for localhost)",
		"http://localhost.evil.com":  "the URL must use https (http is allowed only for localhost)",
		"https://user:pass@host.com": "the URL cannot contain user info",
		"https://example.com?x=1":    "the URL cannot contain a query",
		"https://example.com#x":      "the URL cannot contain a fragment",
		"https:///path":              "the URL needs a host",
		"https://:8443":              "the URL needs a host",
		"ftp://example.com":          "unsupported URL scheme",
	}
	for raw, want := range invalid {
		err := httptool.ValidateURL(raw)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateURL(%q) error = %v, want %q", raw, err, want)
		}
	}
}

func TestBuildGetRefusesDotDot(t *testing.T) {
	svc := &config.Service{URLs: map[string]string{"api": "https://example.com/api"}}
	_, err := httptool.BuildGet(context.Background(), httptool.GetInput{
		Service: svc,
		URL:     "api",
		Path:    "/../secret",
	})
	if err == nil || !strings.Contains(err.Error(), ".. segment") {
		t.Fatalf("BuildGet() error = %v, want a .. segment error", err)
	}
}

func TestSecretHeadersPrevailOverServiceHeaders(t *testing.T) {
	svc := apiService()
	svc.Headers = map[string]string{"X-Key": "service"}
	tool := config.CommandTool{
		Name: "call",
		HTTP: &config.HTTPRequest{
			URL:     "api",
			Method:  http.MethodGet,
			Path:    "/",
			Headers: map[string]string{"X-Key": "tool"},
		},
	}
	request, err := httptool.Build(context.Background(), httptool.Input{
		Tool:          tool,
		Service:       svc,
		SecretHeaders: map[string]string{"X-Key": "secret"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := request.Header.Get("X-Key"); got != "secret" {
		t.Errorf("X-Key = %q, want secret", got)
	}
}

func TestBuildGetBuildsTheURLQueryAndHeaders(t *testing.T) {
	svc := &config.Service{
		URLs:    map[string]string{"api": "https://api.example.com/v1"},
		Headers: map[string]string{"X-Tenant": "{{env.TENANT}}"},
	}
	request, err := httptool.BuildGet(context.Background(), httptool.GetInput{
		Service:       svc,
		Env:           map[string]string{"TENANT": "acme"},
		SecretHeaders: map[string]string{"Authorization": "Bearer token"},
		URL:           "api",
		Path:          "/items",
		Query:         map[string]string{"limit": "10", "page": "2"},
	})
	if err != nil {
		t.Fatalf("BuildGet() error = %v", err)
	}
	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	if got := request.URL.String(); got != "https://api.example.com/v1/items?limit=10&page=2" {
		t.Errorf("URL = %q", got)
	}
	if got := request.Header.Get("X-Tenant"); got != "acme" {
		t.Errorf("X-Tenant = %q, want acme", got)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer token" {
		t.Errorf("Authorization = %q, want the secret header", got)
	}
}

func TestAFormBodyIsEncoded(t *testing.T) {
	svc := apiService()
	tool := config.CommandTool{
		Name: "create",
		HTTP: &config.HTTPRequest{
			URL:    "api",
			Method: http.MethodPost,
			Path:   "/items",
			Form:   map[string]string{"name": "{{name}}", "fixed": "yes"},
		},
		Params: map[string]config.CommandParam{"name": {Type: "string"}},
	}
	request, err := httptool.Build(context.Background(), httptool.Input{
		Tool:    tool,
		Service: svc,
		Args:    map[string]any{"name": "Ana"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if got := string(data); got != "fixed=yes&name=Ana" {
		t.Errorf("body = %q, want fixed=yes&name=Ana", got)
	}
	if got := request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestValidateHeaderRejectsArgumentsAndForbiddenNames(t *testing.T) {
	if err := httptool.ValidateHeader("X-Key", "{{param}}"); err == nil || err.Error() != "service headers can only use {{env.NAME}}" {
		t.Errorf("ValidateHeader() error = %v, want the service header error", err)
	}
	for _, name := range []string{"Content-Type", "host", "Content-Length"} {
		if err := httptool.ValidateHeader(name, "value"); err == nil {
			t.Errorf("ValidateHeader(%q) accepted a forbidden header", name)
		}
	}
	if err := httptool.ValidateHeader("X Key", "value"); err == nil {
		t.Error("ValidateHeader() accepted a header name with a space")
	}
}

func TestValidateRejectsJSONKeysThatConflictOrHaveEmptySegments(t *testing.T) {
	tests := []config.CommandTool{
		{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodPost, Path: "/", JSON: map[string]string{"a": "x", "a.b": "y"}}},
		{Name: "call", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodPost, Path: "/", JSON: map[string]string{"a..b": "x"}}},
	}
	for _, tool := range tests {
		if err := httptool.Validate(tool, apiService(), nil); err == nil {
			t.Errorf("Validate() accepted JSON keys %v", tool.HTTP.JSON)
		}
	}
}

func TestHostsReturnsEveryServiceHost(t *testing.T) {
	svc := &config.Service{URLs: map[string]string{
		"a": "https://a.example.com:8443/path",
		"b": "http://127.0.0.1:1234",
	}}
	hosts := httptool.Hosts(svc)
	for _, want := range []string{"a.example.com:8443", "127.0.0.1:1234"} {
		if !hosts[want] {
			t.Errorf("Hosts() = %v, want %q", hosts, want)
		}
	}
}

func TestAnArgumentInThePathCannotClimbOutOfTheBasePath(t *testing.T) {
	svc := &config.Service{URLs: map[string]string{"api": "https://api.example.com/v1"}}
	tool := config.CommandTool{
		Name:   "item",
		HTTP:   &config.HTTPRequest{Method: http.MethodGet, Path: "/items/{{id}}"},
		Params: map[string]config.CommandParam{"id": {Type: "string"}},
	}
	for _, id := range []string{"..", "."} {
		_, err := httptool.Build(context.Background(), httptool.Input{
			Tool:    tool,
			Service: svc,
			Args:    map[string]any{"id": id},
		})
		if err == nil {
			t.Errorf("Build() with id %q error = nil, want an error", id)
		}
	}
}
