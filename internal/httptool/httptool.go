package httptool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
)

const GetTool = "http_get"

type Input struct {
	Tool          config.CommandTool
	Service       *config.Service
	Env           map[string]string
	SecretEnv     map[string]bool
	SecretHeaders map[string]string
	Args          map[string]any
}

type GetInput struct {
	Service       *config.Service
	Env           map[string]string
	SecretHeaders map[string]string
	URL           string
	Path          string
	Query         map[string]string
}

func ValidateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid URL: check the scheme and host")
	}
	if parsed.User != nil {
		return errors.New("the URL cannot contain user info")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return errors.New("the URL cannot contain a query")
	}
	if strings.Contains(raw, "#") || parsed.Fragment != "" {
		return errors.New("the URL cannot contain a fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q: use https", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return errors.New("the URL needs a host")
	}
	if scheme == "http" {
		switch strings.ToLower(parsed.Hostname()) {
		case "localhost", "127.0.0.1", "::1":
		default:
			return errors.New("the URL must use https (http is allowed only for localhost)")
		}
	}
	return nil
}

func ValidateHeader(name, template string) error {
	if err := validateHeaderName(name); err != nil {
		return err
	}
	parts, err := parseTemplate(template)
	if err != nil {
		return err
	}
	for _, p := range parts {
		if p.kind == paramPart {
			return errors.New("service headers can only use {{env.NAME}}")
		}
	}
	return nil
}

func Validate(tool config.CommandTool, svc *config.Service, secretEnv map[string]bool) error {
	if tool.HTTP == nil {
		return fmt.Errorf("tool %s: HTTP request is missing", tool.Name)
	}
	if len(tool.Command) != 0 {
		return fmt.Errorf("tool %s: use a command or an HTTP request, not both", tool.Name)
	}
	if svc == nil {
		return fmt.Errorf("tool %s: service is missing", tool.Name)
	}
	spec := tool.HTTP
	switch spec.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("tool %s: method must be GET, HEAD, POST, PUT, PATCH or DELETE", tool.Name)
	}
	base, err := selectURL(spec.URL, svc)
	if err != nil {
		return err
	}
	if err := ValidateURL(base); err != nil {
		return err
	}
	if err := validatePath(spec.Path); err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}
	if err := validateLiteralKeys(tool.Name, "query", spec.Query); err != nil {
		return err
	}
	if err := validateLiteralKeys(tool.Name, "form", spec.Form); err != nil {
		return err
	}
	if err := validateJSONKeys(tool.Name, spec.JSON); err != nil {
		return err
	}
	if err := validateToolHeaders(tool, secretEnv); err != nil {
		return err
	}
	if err := validateServiceHeaders(svc); err != nil {
		return err
	}
	if len(spec.Form) > 0 && len(spec.JSON) > 0 {
		return fmt.Errorf("tool %s: use form or json, not both", tool.Name)
	}
	if (spec.Method == http.MethodGet || spec.Method == http.MethodHead) && (len(spec.Form) > 0 || len(spec.JSON) > 0) {
		return fmt.Errorf("tool %s: %s cannot have a form or JSON body", tool.Name, spec.Method)
	}
	if err := validateTemplate(tool.Name, tool.Params, secretEnv, spec.Path, "path"); err != nil {
		return err
	}
	for _, key := range sortedKeys(spec.Query) {
		if err := validateTemplate(tool.Name, tool.Params, secretEnv, spec.Query[key], fmt.Sprintf("query key %q", key)); err != nil {
			return err
		}
	}
	for _, key := range sortedKeys(spec.Form) {
		if err := validateTemplate(tool.Name, tool.Params, secretEnv, spec.Form[key], fmt.Sprintf("form key %q", key)); err != nil {
			return err
		}
	}
	for _, key := range sortedKeys(spec.JSON) {
		if err := validateTemplate(tool.Name, tool.Params, secretEnv, spec.JSON[key], fmt.Sprintf("JSON key %q", key)); err != nil {
			return err
		}
	}
	return nil
}

func Build(ctx context.Context, in Input) (*http.Request, error) {
	if err := Validate(in.Tool, in.Service, in.SecretEnv); err != nil {
		return nil, err
	}
	spec := in.Tool.HTTP
	base, err := selectURL(spec.URL, in.Service)
	if err != nil {
		return nil, err
	}

	pathParts, err := parseTemplate(spec.Path)
	if err != nil {
		return nil, fmt.Errorf("tool %s: %w", in.Tool.Name, err)
	}
	path, missing, err := expandTemplate(pathParts, in.Args, in.Env, url.PathEscape)
	if err != nil {
		return nil, err
	}
	if missing != "" {
		return nil, fmt.Errorf("missing argument %s", missing)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return nil, errors.New("a value in the path cannot be . or ..")
		}
	}

	query := url.Values{}
	for _, key := range sortedKeys(spec.Query) {
		parts, err := parseTemplate(spec.Query[key])
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", in.Tool.Name, err)
		}
		value, missing, err := expandTemplate(parts, in.Args, in.Env, nil)
		if err != nil {
			return nil, err
		}
		if missing != "" {
			continue
		}
		query.Add(key, value)
	}
	finalURL := strings.TrimRight(base, "/") + path
	if encoded := query.Encode(); encoded != "" {
		finalURL += "?" + encoded
	}

	var body io.Reader
	contentType := ""
	if len(spec.Form) > 0 {
		form := url.Values{}
		for _, key := range sortedKeys(spec.Form) {
			parts, err := parseTemplate(spec.Form[key])
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", in.Tool.Name, err)
			}
			value, missing, err := expandTemplate(parts, in.Args, in.Env, nil)
			if err != nil {
				return nil, err
			}
			if missing != "" {
				continue
			}
			form.Add(key, value)
		}
		body = bytes.NewReader([]byte(form.Encode()))
		contentType = "application/x-www-form-urlencoded"
	}
	if len(spec.JSON) > 0 {
		object := map[string]any{}
		for _, key := range sortedKeys(spec.JSON) {
			parts, err := parseTemplate(spec.JSON[key])
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", in.Tool.Name, err)
			}
			if len(parts) == 1 && parts[0].kind == paramPart {
				value, ok := in.Args[parts[0].name]
				if !ok || value == nil {
					continue
				}
				setJSON(object, key, value)
				continue
			}
			value, missing, err := expandTemplate(parts, in.Args, in.Env, nil)
			if err != nil {
				return nil, err
			}
			if missing != "" {
				continue
			}
			setJSON(object, key, value)
		}
		data, err := json.Marshal(object)
		if err != nil {
			return nil, fmt.Errorf("could not encode the JSON body: %w", err)
		}
		body = bytes.NewReader(data)
		contentType = "application/json"
	}

	request, err := http.NewRequestWithContext(ctx, spec.Method, finalURL, body)
	if err != nil {
		return nil, errors.New("invalid URL: check the service URL and path")
	}
	for _, name := range sortedKeys(spec.Headers) {
		parts, err := parseTemplate(spec.Headers[name])
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", in.Tool.Name, err)
		}
		value, missing, err := expandTemplate(parts, in.Args, in.Env, nil)
		if err != nil {
			return nil, err
		}
		if missing != "" {
			continue
		}
		request.Header.Set(name, value)
	}
	if err := applyServiceHeaders(request.Header, in.Service, in.Env); err != nil {
		return nil, err
	}
	applySecretHeaders(request.Header, in.SecretHeaders)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request, nil
}

func BuildGet(ctx context.Context, in GetInput) (*http.Request, error) {
	if in.Service == nil {
		return nil, errors.New("service is missing")
	}
	if in.URL == "" && len(in.Service.URLs) > 1 {
		return nil, fmt.Errorf("the service has several URLs: pass url as one of %s", strings.Join(sortedKeys(in.Service.URLs), ", "))
	}
	base, err := selectURL(in.URL, in.Service)
	if err != nil {
		return nil, err
	}
	if err := ValidateURL(base); err != nil {
		return nil, err
	}
	if err := validatePath(in.Path); err != nil {
		return nil, err
	}
	finalURL := strings.TrimRight(base, "/") + in.Path
	parsed, err := url.Parse(finalURL)
	if err != nil {
		return nil, errors.New("invalid URL: check the service URL and path")
	}
	query := url.Values{}
	for key, value := range in.Query {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, errors.New("invalid URL: check the service URL and path")
	}
	if err := applyServiceHeaders(request.Header, in.Service, in.Env); err != nil {
		return nil, err
	}
	applySecretHeaders(request.Header, in.SecretHeaders)
	return request, nil
}

func Hosts(svc *config.Service) map[string]bool {
	hosts := map[string]bool{}
	if svc == nil {
		return hosts
	}
	for _, raw := range svc.URLs {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			continue
		}
		hosts[parsed.Host] = true
	}
	return hosts
}

func selectURL(name string, svc *config.Service) (string, error) {
	if svc == nil {
		return "", errors.New("service has no URL: add one with pandora http url")
	}
	if len(svc.URLs) == 0 {
		return "", errors.New("service has no URL: add one with pandora http url")
	}
	if name != "" {
		base, ok := svc.URLs[name]
		if !ok {
			return "", fmt.Errorf("unknown URL %q: add it with pandora http url", name)
		}
		return base, nil
	}
	if len(svc.URLs) != 1 {
		return "", errors.New("the service has several URLs: choose one with --url")
	}
	for _, base := range svc.URLs {
		return base, nil
	}
	return "", errors.New("service has no URL: add one with pandora http url")
}

func validateHeaderName(name string) error {
	if name == "" {
		return errors.New("header name is empty")
	}
	for i := 0; i < len(name); i++ {
		if !isTChar(name[i]) {
			return fmt.Errorf("invalid header name %q: use a valid HTTP token", name)
		}
	}
	switch strings.ToLower(name) {
	case "host", "content-length", "content-type", "transfer-encoding", "connection":
		return fmt.Errorf("header %s cannot be set", name)
	}
	return nil
}

func isTChar(c byte) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

func validatePath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return errors.New("path must start with /")
	}
	if strings.ContainsAny(path, "?#") {
		return errors.New("path cannot contain ? or #")
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return errors.New("path cannot contain a .. segment")
		}
	}
	return nil
}

func validateLiteralKeys(toolName, field string, values map[string]string) error {
	for _, key := range sortedKeys(values) {
		if key == "" {
			return fmt.Errorf("tool %s: %s key cannot be empty", toolName, field)
		}
		if strings.Contains(key, "{{") {
			return fmt.Errorf("tool %s: %s key %q must be literal", toolName, field, key)
		}
	}
	return nil
}

func validateJSONKeys(toolName string, values map[string]string) error {
	keys := sortedKeys(values)
	for _, key := range keys {
		if key == "" {
			return fmt.Errorf("tool %s: JSON key cannot be empty", toolName)
		}
		if strings.Contains(key, "{{") {
			return fmt.Errorf("tool %s: JSON key %q must be literal", toolName, key)
		}
		for _, segment := range strings.Split(key, ".") {
			if segment == "" {
				return fmt.Errorf("tool %s: JSON key %q has an empty segment", toolName, key)
			}
		}
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if strings.HasPrefix(keys[j], keys[i]+".") {
				return fmt.Errorf("tool %s: JSON keys %q and %q conflict", toolName, keys[i], keys[j])
			}
		}
	}
	return nil
}

func validateToolHeaders(tool config.CommandTool, secretEnv map[string]bool) error {
	for _, name := range sortedKeys(tool.HTTP.Headers) {
		if err := validateHeaderName(name); err != nil {
			return fmt.Errorf("tool %s: %w", tool.Name, err)
		}
		if err := validateTemplate(tool.Name, tool.Params, secretEnv, tool.HTTP.Headers[name], fmt.Sprintf("header %q", name)); err != nil {
			return err
		}
	}
	return nil
}

func validateServiceHeaders(svc *config.Service) error {
	for _, name := range sortedKeys(svc.Headers) {
		if err := ValidateHeader(name, svc.Headers[name]); err != nil {
			return err
		}
	}
	return nil
}

func validateTemplate(toolName string, params map[string]config.CommandParam, secretEnv map[string]bool, raw, field string) error {
	parts, err := parseTemplate(raw)
	if err != nil {
		return fmt.Errorf("tool %s: %w", toolName, err)
	}
	for _, p := range parts {
		switch p.kind {
		case paramPart:
			if _, ok := params[p.name]; !ok {
				return fmt.Errorf("tool %s: unknown parameter %s in %s", toolName, p.name, field)
			}
		case envPart:
			if secretEnv[p.name] {
				return fmt.Errorf("tool %s: the secret %s can only be used in the service headers", toolName, p.name)
			}
		}
	}
	return nil
}

func applyServiceHeaders(header http.Header, svc *config.Service, env map[string]string) error {
	for _, name := range sortedKeys(svc.Headers) {
		if err := ValidateHeader(name, svc.Headers[name]); err != nil {
			return err
		}
		parts, err := parseTemplate(svc.Headers[name])
		if err != nil {
			return err
		}
		value, missing, err := expandTemplate(parts, nil, env, nil)
		if err != nil {
			return err
		}
		if missing != "" {
			return errors.New("service headers can only use {{env.NAME}}")
		}
		header.Set(name, value)
	}
	return nil
}

func applySecretHeaders(header http.Header, values map[string]string) {
	for _, name := range sortedKeys(values) {
		header.Set(name, values[name])
	}
}

func setJSON(root map[string]any, path string, value any) {
	segments := strings.Split(path, ".")
	current := root
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
	current[segments[len(segments)-1]] = value
}
