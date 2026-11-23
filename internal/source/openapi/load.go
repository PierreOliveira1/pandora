package openapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/PierreOliveira1/pandora/internal/source"
)

const (
	maxSpecSize = 10 * 1024 * 1024
	maxRefDepth = 12
)

type loadedSpec struct {
	root    map[string]any
	version int
	specURL *url.URL
}

func load(ctx context.Context, spec source.Spec) (*loadedSpec, error) {
	data, specURL, err := readSpec(ctx, spec)
	if err != nil {
		return nil, err
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI spec %s: %w", spec.Svc.Spec, err)
	}
	root, ok := normalizeYAML(raw).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parsing OpenAPI spec %s: expected an object at the top level", spec.Svc.Spec)
	}
	version, err := specVersion(root)
	if err != nil {
		return nil, err
	}
	resolved, ok := resolveValue(root, root, nil, 0).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parsing OpenAPI spec %s: expected an object at the top level", spec.Svc.Spec)
	}
	return &loadedSpec{root: resolved, version: version, specURL: specURL}, nil
}

func readSpec(ctx context.Context, spec source.Spec) ([]byte, *url.URL, error) {
	name := spec.Svc.Spec
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		client := spec.HTTPClient
		if client == nil {
			client = http.DefaultClient
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, name, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("loading OpenAPI spec %s: %w", name, err)
		}
		for header, value := range spec.Headers {
			request.Header.Set(header, value)
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, nil, fmt.Errorf("loading OpenAPI spec %s: %w", name, err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode > 299 {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			return nil, nil, fmt.Errorf("loading OpenAPI spec %s: HTTP %s", name, response.Status)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, maxSpecSize+1))
		if err != nil {
			return nil, nil, fmt.Errorf("loading OpenAPI spec %s: %w", name, err)
		}
		if len(data) > maxSpecSize {
			return nil, nil, fmt.Errorf("OpenAPI spec %s is larger than 10 MB", name)
		}
		return data, response.Request.URL, nil
	}

	info, err := os.Stat(name)
	if err != nil {
		return nil, nil, fmt.Errorf("reading OpenAPI spec %s: %w", name, err)
	}
	if info.Size() > maxSpecSize {
		return nil, nil, fmt.Errorf("OpenAPI spec %s is larger than 10 MB", name)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, fmt.Errorf("reading OpenAPI spec %s: %w", name, err)
	}
	if len(data) > maxSpecSize {
		return nil, nil, fmt.Errorf("OpenAPI spec %s is larger than 10 MB", name)
	}
	return data, nil, nil
}

func specVersion(root map[string]any) (int, error) {
	if value, ok := root["openapi"]; ok {
		text := fmt.Sprint(value)
		if strings.HasPrefix(text, "3.") || text == "3" {
			return 3, nil
		}
	}
	if value, ok := root["swagger"]; ok {
		text := fmt.Sprint(value)
		if text == "2.0" || text == "2" {
			return 2, nil
		}
	}
	return 0, errors.New("unsupported spec: only OpenAPI 3.x and Swagger 2.0 are supported")
}

func normalizeYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeYAML(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = normalizeYAML(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeYAML(item)
		}
		return out
	default:
		return value
	}
}

func resolveValue(value any, root map[string]any, stack []string, depth int) any {
	switch typed := value.(type) {
	case map[string]any:
		if reference, ok := typed["$ref"].(string); ok {
			if depth >= maxRefDepth || !strings.HasPrefix(reference, "#/") || containsRef(stack, reference) {
				return objectSchema()
			}
			target, ok := lookupPointer(root, reference[2:])
			if !ok {
				return objectSchema()
			}
			next := append(append([]string(nil), stack...), reference)
			return resolveValue(target, root, next, depth+1)
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = resolveValue(item, root, stack, depth)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = resolveValue(item, root, stack, depth)
		}
		return out
	default:
		return value
	}
}

func containsRef(stack []string, reference string) bool {
	for _, item := range stack {
		if item == reference {
			return true
		}
	}
	return false
}

func lookupPointer(root map[string]any, pointer string) (any, bool) {
	decoded, err := url.PathUnescape(pointer)
	if err != nil {
		return nil, false
	}
	if decoded == "" {
		return root, true
	}
	var current any = root
	for _, token := range strings.Split(decoded, "/") {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		switch node := current.(type) {
		case map[string]any:
			next, ok := node[token]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func objectSchema() map[string]any {
	return map[string]any{"type": "object"}
}

func baseURL(doc *loadedSpec, override string) (string, error) {
	if override != "" {
		return normalizeBaseURL(override)
	}
	if doc.version == 3 {
		return openAPI3BaseURL(doc)
	}
	return swaggerBaseURL(doc.root)
}

func openAPI3BaseURL(doc *loadedSpec) (string, error) {
	servers, _ := doc.root["servers"].([]any)
	if len(servers) == 0 {
		return "", noBaseURL()
	}
	server, _ := servers[0].(map[string]any)
	raw, _ := server["url"].(string)
	if raw == "" {
		return "", noBaseURL()
	}
	raw = substituteServerVariables(raw, server)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid OpenAPI server URL %q: %w", raw, err)
	}
	if !parsed.IsAbs() {
		if doc.specURL == nil {
			return "", fmt.Errorf("the OpenAPI server URL %q is relative: add the account again with --url <api base url>", raw)
		}
		parsed = doc.specURL.ResolveReference(parsed)
	}
	return normalizeBaseURL(parsed.String())
}

func substituteServerVariables(raw string, server map[string]any) string {
	variables, _ := server["variables"].(map[string]any)
	for name, value := range variables {
		definition, _ := value.(map[string]any)
		if definition == nil {
			continue
		}
		defaultValue, ok := definition["default"]
		if !ok {
			continue
		}
		raw = strings.ReplaceAll(raw, "{"+name+"}", fmt.Sprint(defaultValue))
	}
	return raw
}

func swaggerBaseURL(root map[string]any) (string, error) {
	host, _ := root["host"].(string)
	if host == "" {
		return "", noBaseURL()
	}
	scheme := "https"
	if schemes, ok := root["schemes"].([]any); ok && len(schemes) > 0 {
		if value, ok := schemes[0].(string); ok && value != "" {
			scheme = value
		}
	}
	basePath, _ := root["basePath"].(string)
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return normalizeBaseURL(scheme + "://" + host + basePath)
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("invalid base URL %q: use an absolute http or https URL", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func noBaseURL() error {
	return errors.New("no base URL in the spec: add the account again with --url <api base url>")
}
