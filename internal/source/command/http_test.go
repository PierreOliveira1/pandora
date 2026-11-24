package command

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/source"
)

type httpTestCapture struct {
	method string
	path   string
	query  string
	body   map[string]any
	err    error
}

func openHTTPTestSource(t *testing.T, spec source.Spec) *Source {
	t.Helper()
	src, err := Open(context.Background(), spec)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return src.(*Source)
}

func TestAnHTTPToolSendsTheConfiguredRequestAndReturnsTheBody(t *testing.T) {
	captured := make(chan httpTestCapture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		var body map[string]any
		if err == nil && len(data) > 0 {
			err = json.Unmarshal(data, &body)
		}
		captured <- httpTestCapture{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			body:   body,
			err:    err,
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{"api": server.URL},
		Tools: []config.CommandTool{{
			Name: "create",
			HTTP: &config.HTTPRequest{
				URL:    "api",
				Method: http.MethodPost,
				Path:   "/items",
				Query:  map[string]string{"q": "{{term}}"},
				JSON:   map[string]string{"name": "{{name}}", "n": "{{n}}"},
			},
			Params: map[string]config.CommandParam{
				"term": {Type: "string"},
				"name": {Type: "string"},
				"n":    {Type: "integer"},
			},
		}},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})

	result, err := src.CallTool(context.Background(), "create", map[string]any{
		"term": "x",
		"name": "Ana",
		"n":    3,
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	if got := strings.TrimSpace(resultText(result)); got != "ok" {
		t.Errorf("output = %q, want ok", got)
	}
	got := <-captured
	if got.err != nil {
		t.Fatalf("reading request body: %v", got.err)
	}
	if got.method != http.MethodPost || got.path != "/items" || got.query != "q=x" {
		t.Fatalf("request = %s %s?%s", got.method, got.path, got.query)
	}
	if got.body["name"] != "Ana" || got.body["n"] != float64(3) {
		t.Fatalf("JSON body = %v", got.body)
	}
}

func TestAServiceHeaderUsesTheAccountSecret(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	svc := &config.Service{
		Kind:    config.KindHTTP,
		URLs:    map[string]string{"api": server.URL},
		Headers: map[string]string{"Authorization": "OAuth {{env.TOKEN}}"},
		Tools: []config.CommandTool{{
			Name: "me",
			HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/me"},
		}},
	}
	src := openHTTPTestSource(t, source.Spec{
		Svc: svc,
		Acc: &config.Account{SecretEnv: []string{"TOKEN"}},
		Env: map[string]string{"TOKEN": "abc"},
	})

	result, err := src.CallTool(context.Background(), "me", nil)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	if got := <-authorization; got != "OAuth abc" {
		t.Fatalf("Authorization = %q, want OAuth abc", got)
	}
}

func TestAnHTTPErrorStatusBecomesAToolError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{"api": server.URL},
		Tools: []config.CommandTool{{
			Name: "fail",
			HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/fail"},
		}},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})

	result, err := src.CallTool(context.Background(), "fail", nil)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() IsError = false, want true")
	}
	text := resultText(result)
	if !strings.Contains(text, "HTTP 400 Bad Request") || !strings.Contains(text, "bad request") {
		t.Fatalf("result = %q, want the status and body", text)
	}
}

func TestARedirectToAnotherHostIsRefused(t *testing.T) {
	var secondHits atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		_, _ = w.Write([]byte("second"))
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+"/target", http.StatusFound)
	}))
	defer first.Close()

	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{"api": first.URL},
		Tools: []config.CommandTool{{
			Name: "redirect",
			HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/start"},
		}},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})

	result, err := src.CallTool(context.Background(), "redirect", nil)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError || resultText(result) != "redirect to another host refused" {
		t.Fatalf("result = %q, want the redirect refusal", resultText(result))
	}
	if secondHits.Load() != 0 {
		t.Fatalf("the second server received %d requests, want none", secondHits.Load())
	}
}

func TestHTTPGetReadsTheChosenURLAndRejectsUnknownNames(t *testing.T) {
	type request struct {
		path  string
		query string
	}
	firstHits := make(chan request, 1)
	secondHits := make(chan request, 1)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits <- request{path: r.URL.Path, query: r.URL.RawQuery}
		_, _ = w.Write([]byte("first"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits <- request{path: r.URL.Path, query: r.URL.RawQuery}
		_, _ = w.Write([]byte("second"))
	}))
	defer second.Close()

	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{"a": first.URL, "b": second.URL},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})

	result, err := src.CallTool(context.Background(), "http_get", map[string]any{
		"url":   "b",
		"path":  "/items",
		"query": map[string]any{"q": "x"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	if got := strings.TrimSpace(resultText(result)); got != "second" {
		t.Fatalf("output = %q, want second", got)
	}
	got := <-secondHits
	if got.path != "/items" || got.query != "q=x" {
		t.Fatalf("second request = %s?%s", got.path, got.query)
	}
	select {
	case <-firstHits:
		t.Fatal("the first server received the request")
	default:
	}

	unknown, err := src.CallTool(context.Background(), "http_get", map[string]any{
		"url":  "missing",
		"path": "/items",
	})
	if err != nil {
		t.Fatalf("unknown URL error = %v", err)
	}
	if !unknown.IsError || !strings.Contains(resultText(unknown), "unknown URL") {
		t.Fatalf("unknown URL result = %q", resultText(unknown))
	}

	badQuery, err := src.CallTool(context.Background(), "http_get", map[string]any{
		"path":  "/items",
		"query": map[string]any{"q": 1},
	})
	if err != nil {
		t.Fatalf("bad query error = %v", err)
	}
	if !badQuery.IsError || resultText(badQuery) != "query values must be strings" {
		t.Fatalf("bad query result = %q", resultText(badQuery))
	}
}

func TestHTTPGetToolsAreMarkedReadOnly(t *testing.T) {
	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{"api": "https://api.example.com"},
		Tools: []config.CommandTool{
			{Name: "read", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodGet, Path: "/read"}},
			{Name: "write", HTTP: &config.HTTPRequest{URL: "api", Method: http.MethodPost, Path: "/write"}},
		},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})
	tools, err := src.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	foundRead := false
	foundWrite := false
	for _, tool := range tools {
		switch tool.Name {
		case "read":
			foundRead = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Fatal("GET tool is not marked read-only")
			}
		case "write":
			foundWrite = true
			if tool.Annotations != nil && tool.Annotations.ReadOnlyHint {
				t.Fatal("POST tool is marked read-only")
			}
		}
	}
	if !foundRead || !foundWrite {
		t.Fatalf("tools = %v, want read and write", tools)
	}
}

func TestHTTPGetDescribesTheAvailableURLsAndSchema(t *testing.T) {
	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{
			"z": "https://z.example.com",
			"a": "https://a.example.com",
		},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})
	tools, err := src.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var get *mcp.Tool
	for _, tool := range tools {
		if tool.Name == "http_get" {
			get = tool
		}
	}
	if get == nil {
		t.Fatal("ListTools() did not include http_get")
	}
	if !strings.Contains(get.Description, "a (https://a.example.com), z (https://z.example.com)") {
		t.Fatalf("description = %q", get.Description)
	}
	schema, ok := get.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema = %T", get.InputSchema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %v", schema["properties"])
	}
	if properties["path"].(map[string]any)["type"] != "string" {
		t.Fatalf("path schema = %v", properties["path"])
	}
	urlSchema, ok := properties["url"].(map[string]any)
	if !ok {
		t.Fatalf("url schema = %v", properties["url"])
	}
	enum, ok := urlSchema["enum"].([]string)
	if !ok || len(enum) != 2 || enum[0] != "a" || enum[1] != "z" {
		t.Fatalf("url enum = %v", urlSchema["enum"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "path" || required[1] != "url" {
		t.Fatalf("required = %v", schema["required"])
	}
}

func TestAProgramToolStillRunsInAnHTTPService(t *testing.T) {
	svc := &config.Service{
		Kind: config.KindHTTP,
		URLs: map[string]string{"api": "https://api.example.com"},
		Tools: []config.CommandTool{{
			Name:    "echo",
			Command: []string{"/bin/echo", "hello", "{{name}}"},
			Params: map[string]config.CommandParam{
				"name": {Type: "string", Required: true},
			},
		}},
	}
	src := openHTTPTestSource(t, source.Spec{Svc: svc})

	result, err := src.CallTool(context.Background(), "echo", map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	if got := strings.TrimSpace(resultText(result)); got != "hello world" {
		t.Fatalf("output = %q, want hello world", got)
	}
}

func TestAnHTTPToolWithoutServiceURLsIsRefused(t *testing.T) {
	svc := &config.Service{
		Kind: config.KindCommand,
		Tools: []config.CommandTool{{
			Name: "call",
			HTTP: &config.HTTPRequest{Method: http.MethodGet, Path: "/"},
		}},
	}
	_, err := Open(context.Background(), source.Spec{Svc: svc})
	if err == nil || !strings.Contains(err.Error(), "service has no URL") {
		t.Fatalf("Open() error = %v, want a missing URL error", err)
	}
}
