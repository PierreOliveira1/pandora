package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
)

type redirectRefusedError struct{}

func (redirectRefusedError) Error() string {
	return "redirect to another host refused"
}

func newHTTPClient(base *http.Client, svc *config.Service) *http.Client {
	hosts := httptool.Hosts(svc)
	client := &http.Client{}
	if base != nil {
		copied := *base
		client = &copied
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if !hosts[req.URL.Host] {
			return redirectRefusedError{}
		}
		return nil
	}
	return client
}

func secretEnvSet(acc *config.Account) map[string]bool {
	names := map[string]bool{}
	if acc == nil {
		return names
	}
	for _, name := range acc.SecretEnv {
		names[name] = true
	}
	return names
}

func httpToolReadOnly(tool config.CommandTool) bool {
	return tool.HTTP != nil && (tool.HTTP.Method == http.MethodGet || tool.HTTP.Method == http.MethodHead)
}

func httpGetTool(svc *config.Service) *mcp.Tool {
	names := sortedURLNames(svc.URLs)
	urls := make([]string, len(names))
	for i, name := range names {
		urls[i] = name + " (" + svc.URLs[name] + ")"
	}
	required := []string{"path"}
	if len(names) > 1 {
		required = append(required, "url")
	}
	return &mcp.Tool{
		Name:        httptool.GetTool,
		Description: "Read-only GET request to this service's API, to explore it before adding a tool with pandora tool add --http. URLs: " + strings.Join(urls, ", "),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Path after the base URL, starting with /"},
				"url":  map[string]any{"type": "string", "enum": names, "description": "Name of the base URL to call"},
				"query": map[string]any{
					"type":                 "object",
					"description":          "Query parameters as name to value",
					"additionalProperties": map[string]any{"type": "string"},
				},
			},
			"required": required,
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func sortedURLNames(urls map[string]string) []string {
	names := make([]string, 0, len(urls))
	for name := range urls {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Source) callHTTPTool(ctx context.Context, tool config.CommandTool, args map[string]any) (*mcp.CallToolResult, error) {
	_, original, failure := validateArguments(tool.Params, args)
	if failure != nil {
		return failure, nil
	}
	request, err := httptool.Build(ctx, httptool.Input{
		Tool:          tool,
		Service:       s.svc,
		Env:           s.env,
		SecretEnv:     s.secretEnv,
		SecretHeaders: s.secretHeaders,
		Args:          original,
	})
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return s.executeHTTP(ctx, request, tool.TimeoutSec), nil
}

func (s *Source) callHTTPGet(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	pathValue, ok := args["path"]
	if !ok || pathValue == nil {
		return errorResult("missing required argument path"), nil
	}
	path, ok := pathValue.(string)
	if !ok {
		return errorResult("argument path must be a string"), nil
	}
	if !strings.HasPrefix(path, "/") {
		return errorResult("path must start with /"), nil
	}
	urlName := ""
	if value, ok := args["url"]; ok && value != nil {
		text, ok := value.(string)
		if !ok {
			return errorResult("argument url must be a string"), nil
		}
		urlName = text
	}
	query, failure := httpGetQuery(args["query"])
	if failure != nil {
		return failure, nil
	}
	request, err := httptool.BuildGet(ctx, httptool.GetInput{
		Service:       s.svc,
		Env:           s.env,
		SecretHeaders: s.secretHeaders,
		URL:           urlName,
		Path:          path,
		Query:         query,
	})
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return s.executeHTTP(ctx, request, 0), nil
}

func httpGetQuery(value any) (map[string]string, *mcp.CallToolResult) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case map[string]string:
		return cloneEnv(typed), nil
	case map[string]any:
		query := make(map[string]string, len(typed))
		for key, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, errorResult("query values must be strings")
			}
			query[key] = text
		}
		return query, nil
	default:
		return nil, errorResult("query must be an object")
	}
}

func (s *Source) executeHTTP(ctx context.Context, request *http.Request, timeoutSec int) *mcp.CallToolResult {
	timeout := timeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	response, err := s.httpClient.Do(request.WithContext(callCtx))
	if err != nil {
		var refused redirectRefusedError
		if errors.As(err, &refused) {
			return errorResult(refused.Error())
		}
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return errorResult(fmt.Sprintf("timed out after %ds", timeout))
		}
		return errorResult(err.Error())
	}
	defer response.Body.Close()

	body := &limitedBuffer{limit: maxCapture}
	if _, err := io.Copy(body, response.Body); err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return errorResult(fmt.Sprintf("timed out after %ds", timeout))
		}
		return errorResult(err.Error())
	}
	text := body.String()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if text == "" {
			return textResult(fmt.Sprintf("HTTP %d", response.StatusCode))
		}
		return textResult(truncate(text, body.total))
	}
	header := fmt.Sprintf("HTTP %s\n", response.Status)
	return errorResult(truncate(header+text, int64(len(header))+body.total))
}
