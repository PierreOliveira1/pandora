package openapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/source"
)

func TestAnOpenAPISpecBecomesOneToolPerOperationWithItsArguments(t *testing.T) {
	src := loadTestSource(t, "testdata/openapi3.yaml", "", nil)
	tools, err := src.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if src.Instructions() != "Example API\n\nManage users." {
		t.Errorf("Instructions() = %q", src.Instructions())
	}
	byName := toolMap(tools)
	wantNames := []string{"create_node", "create_user", "delete_user", "get_health", "get_user", "submit_form", "update_user"}
	gotNames := make([]string, len(tools))
	for i, tool := range tools {
		gotNames[i] = tool.Name
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("tool names = %v, want %v", gotNames, wantNames)
	}

	update := byName["update_user"]
	if update == nil {
		t.Fatal("update_user tool is missing")
	}
	for _, want := range []string{"Update a user", "Replaces the user.", "HTTP PUT /users/{id}"} {
		if !strings.Contains(update.Description, want) {
			t.Errorf("update_user description does not contain %q: %q", want, update.Description)
		}
	}
	if update.Annotations == nil || update.Annotations.ReadOnlyHint {
		t.Errorf("update_user annotations = %+v, want a non-read-only tool", update.Annotations)
	}
	if update.Annotations != nil && update.Annotations.DestructiveHint != nil {
		t.Error("update_user is marked destructive")
	}
	updateProperties := propertiesOf(t, update)
	for _, name := range []string{"id", "notify", "X-Trace", "body"} {
		if _, ok := updateProperties[name]; !ok {
			t.Errorf("update_user is missing property %q: %v", name, updateProperties)
		}
	}
	if _, ok := updateProperties["Authorization"]; ok {
		t.Error("update_user includes the ignored Authorization header")
	}
	if got := requiredOf(t, update); !reflect.DeepEqual(got, []string{"body", "id"}) {
		t.Errorf("update_user required = %v, want [body id]", got)
	}
	if got := schemaType(t, updateProperties["notify"]); got != "boolean" {
		t.Errorf("notify type = %q, want boolean", got)
	}
	body := schemaOf(t, updateProperties["body"])
	bodyProperties := schemaOf(t, body["properties"])
	if got := schemaType(t, bodyProperties["name"]); got != "string" {
		t.Errorf("body.name type = %q, want string", got)
	}

	get := byName["get_user"]
	if get == nil {
		t.Fatal("get_user tool is missing")
	}
	if get.Annotations == nil || !get.Annotations.ReadOnlyHint {
		t.Errorf("get_user annotations = %+v, want read-only", get.Annotations)
	}
	if got := schemaType(t, propertiesOf(t, get)["verbose"]); got != "boolean" {
		t.Errorf("verbose type = %q, want boolean", got)
	}

	deleteTool := byName["delete_user"]
	if deleteTool == nil {
		t.Fatal("delete_user tool is missing")
	}
	if deleteTool.Annotations == nil || deleteTool.Annotations.DestructiveHint == nil || !*deleteTool.Annotations.DestructiveHint {
		t.Errorf("delete_user annotations = %+v, want destructive", deleteTool.Annotations)
	}

	form := byName["submit_form"]
	if form == nil {
		t.Fatal("submit_form tool is missing")
	}
	formBody := schemaOf(t, propertiesOf(t, form)["body"])
	if formBody["description"] != "Form fields" {
		t.Errorf("form body description = %v, want Form fields", formBody["description"])
	}
	if _, ok := byName["get_health"]; !ok {
		t.Error("get_health fallback tool is missing")
	}
}

func TestASwagger20SpecIsSupported(t *testing.T) {
	src := loadTestSource(t, "testdata/swagger2.json", "", nil)
	if src.baseURL != "https://pets.example.com/v1" {
		t.Errorf("base URL = %q, want https://pets.example.com/v1", src.baseURL)
	}
	tools, err := src.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	byName := toolMap(tools)
	if len(tools) != 2 {
		t.Fatalf("tools = %v, want 2", toolNames(tools))
	}
	list := byName["list_pets"]
	if list == nil {
		t.Fatal("list_pets tool is missing")
	}
	if got := schemaType(t, propertiesOf(t, list)["limit"]); got != "integer" {
		t.Errorf("limit type = %q, want integer", got)
	}
	create := byName["create_pet"]
	if create == nil {
		t.Fatal("create_pet tool is missing")
	}
	if got := requiredOf(t, create); !reflect.DeepEqual(got, []string{"body"}) {
		t.Errorf("create_pet required = %v, want [body]", got)
	}
	body := schemaOf(t, propertiesOf(t, create)["body"])
	bodyProperties := schemaOf(t, body["properties"])
	if got := schemaType(t, bodyProperties["name"]); got != "string" {
		t.Errorf("body.name type = %q, want string", got)
	}
}

func TestCallingAToolSendsThePathQueryBodyAndTheAccountSecretHeaders(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/api/users/42" {
			t.Errorf("path = %q, want /api/users/42", r.URL.Path)
		}
		if got := r.URL.Query().Get("notify"); got != "true" {
			t.Errorf("notify = %q, want true", got)
		}
		if got := r.Header.Get("X-Trace"); got != "trace-1" {
			t.Errorf("X-Trace = %q, want trace-1", got)
		}
		if got := r.Header.Get("X-API-Key"); got != "secret-value" {
			t.Errorf("X-API-Key = %q, want secret-value", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json, */*" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding body: %v", err)
		}
		if body["name"] != "Ada" {
			t.Errorf("body = %v, want name Ada", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"id":"42"}`)
	}))
	defer server.Close()

	src := loadTestSource(t, "testdata/openapi3.yaml", server.URL+"/api", map[string]string{"X-API-Key": "secret-value"})
	result, err := src.CallTool(context.Background(), "update_user", map[string]any{
		"id":      "42",
		"notify":  true,
		"X-Trace": "trace-1",
		"body":    map[string]any{"name": "Ada", "age": 37},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, `"ok": true`) || !strings.Contains(text, `"id": "42"`) {
		t.Errorf("result text = %q, want indented JSON", text)
	}
	if !called.Load() {
		t.Error("the HTTP API was not called")
	}
}

func TestAnHTTPErrorComesBackAsAToolErrorWithTheStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "not found")
	}))
	defer server.Close()

	src := loadTestSource(t, "testdata/openapi3.yaml", server.URL, nil)
	result, err := src.CallTool(context.Background(), "get_user", map[string]any{"id": "1"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() IsError = false, want true")
	}
	text := resultText(result)
	if !strings.HasPrefix(text, "HTTP 404") {
		t.Errorf("result text = %q, want it to start with HTTP 404", text)
	}
	if !strings.Contains(text, "not found") {
		t.Errorf("result text = %q, want the response body", text)
	}
}

func TestARecursiveSchemaDoesNotHangTheLoader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	src, err := Open(ctx, source.Spec{
		Svc: &config.Service{Kind: config.KindOpenAPI, Spec: "testdata/openapi3.yaml"},
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	typed := src.(*Source)
	tools := typed.tools
	byName := toolMap(tools)
	node := byName["create_node"]
	if node == nil {
		t.Fatal("create_node tool is missing")
	}
	body := schemaOf(t, propertiesOf(t, node)["body"])
	children := schemaOf(t, schemaOf(t, body["properties"])["children"])
	items := schemaOf(t, children["items"])
	if got := schemaType(t, items); got != "object" {
		t.Errorf("recursive items type = %q, want object", got)
	}
}

func TestSecretHeadersAreNeverSentToAnotherHostOnRedirect(t *testing.T) {
	var targetCalled atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled.Store(true)
		if got := r.Header.Get("X-API-Key"); got != "" {
			t.Errorf("target received X-API-Key = %q, want no secret", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "secret-value" {
			t.Errorf("origin received X-API-Key = %q, want secret-value", got)
		}
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()

	src := loadTestSource(t, "testdata/openapi3.yaml", origin.URL, map[string]string{"X-API-Key": "secret-value"})
	result, err := src.CallTool(context.Background(), "get_user", map[string]any{"id": "1"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() followed the cross-host redirect")
	}
	want := "refused to follow a redirect to " + strings.TrimPrefix(target.URL, "http://")
	if got := resultText(result); got != want {
		t.Errorf("result text = %q, want %q", got, want)
	}
	if targetCalled.Load() {
		t.Error("the redirect target was called")
	}
}

func loadTestSource(t *testing.T, specPath, override string, headers map[string]string) *Source {
	t.Helper()
	value, err := Open(context.Background(), source.Spec{
		Svc:     &config.Service{Kind: config.KindOpenAPI, Spec: specPath},
		URL:     override,
		Headers: headers,
	})
	if err != nil {
		t.Fatalf("Open(%s) error = %v", specPath, err)
	}
	return value.(*Source)
}

func toolMap(tools []*mcp.Tool) map[string]*mcp.Tool {
	values := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		values[tool.Name] = tool
	}
	return values
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func propertiesOf(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	schema := schemaOf(t, tool.InputSchema)
	return schemaOf(t, schema["properties"])
}

func schemaOf(t *testing.T, value any) map[string]any {
	t.Helper()
	schema, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value %v is not a JSON schema object", value)
	}
	return schema
}

func schemaType(t *testing.T, value any) string {
	t.Helper()
	schema := schemaOf(t, value)
	kind, _ := schema["type"].(string)
	return kind
}

func requiredOf(t *testing.T, tool *mcp.Tool) []string {
	t.Helper()
	schema := schemaOf(t, tool.InputSchema)
	switch value := schema["required"].(type) {
	case []string:
		return value
	case []any:
		names := make([]string, len(value))
		for i, item := range value {
			names[i], _ = item.(string)
		}
		return names
	default:
		return nil
	}
}

func resultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}
