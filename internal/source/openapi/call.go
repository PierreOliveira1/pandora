package openapi

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
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxRead   = 2 * 1024 * 1024
	maxOutput = 100 * 1024
)

func (s *Source) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	operation, ok := s.operations[name]
	if !ok {
		return errorResult("unknown tool " + name), nil
	}
	path := operation.path
	for _, parameter := range operation.pathParams {
		value, ok := args[parameter.property]
		if !ok || value == nil {
			return errorResult("missing required argument " + parameter.property), nil
		}
		path = strings.ReplaceAll(path, "{"+parameter.name+"}", url.PathEscape(fmt.Sprint(value)))
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target, err := url.Parse(strings.TrimRight(s.baseURL, "/") + path)
	if err != nil {
		return errorResult("invalid request URL: " + err.Error()), nil
	}
	query := target.Query()
	for _, parameter := range operation.query {
		value, ok := args[parameter.property]
		if !ok || value == nil {
			continue
		}
		for _, item := range queryValues(value) {
			query.Add(parameter.name, item)
		}
	}
	target.RawQuery = query.Encode()

	var body io.Reader
	contentType := ""
	if operation.hasBody {
		if value, ok := args["body"]; ok && value != nil {
			if operation.form {
				fields, ok := value.(map[string]any)
				if !ok {
					return errorResult("argument body must be an object"), nil
				}
				values := url.Values{}
				for field, item := range fields {
					for _, entry := range queryValues(item) {
						values.Add(field, entry)
					}
				}
				body = strings.NewReader(values.Encode())
				contentType = "application/x-www-form-urlencoded"
			} else {
				data, err := json.Marshal(value)
				if err != nil {
					return errorResult("encoding body: " + err.Error()), nil
				}
				body = bytes.NewReader(data)
				contentType = "application/json"
			}
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(callCtx, strings.ToUpper(operation.method), target.String(), body)
	if err != nil {
		return errorResult("creating request: " + err.Error()), nil
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for _, parameter := range operation.headers {
		value, ok := args[parameter.property]
		if !ok || value == nil || ignoredHeader(parameter.name) {
			continue
		}
		request.Header.Set(parameter.name, headerValue(value))
	}
	for header, value := range s.headers {
		request.Header.Set(header, value)
	}
	request.Header.Set("Accept", "application/json, */*")

	response, err := s.client.Do(request)
	if err != nil {
		var refused *redirectRefusedError
		if errors.As(err, &refused) {
			return errorResult(refused.Error()), nil
		}
		return nil, fmt.Errorf("calling %s: %w", name, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRead+1))
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", name, err)
	}
	total := int64(len(data))
	if response.ContentLength > total {
		total = response.ContentLength
	}
	if len(data) > maxRead {
		data = data[:maxRead]
	}
	text := responseText(data, response.Header.Get("Content-Type"))
	if response.StatusCode >= 400 {
		status := fmt.Sprintf("HTTP %d %s", response.StatusCode, http.StatusText(response.StatusCode))
		if text == "" {
			text = status
		} else {
			text = status + "\n\n" + text
		}
		return errorResult(truncate(text, total)), nil
	}
	return textResult(truncate(text, total)), nil
}

func queryValues(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values := make([]string, len(typed))
		for i, item := range typed {
			values[i] = queryValue(item)
		}
		return values
	default:
		return []string{queryValue(value)}
	}
}

func queryValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case map[string]any, []any, []string:
		data, err := json.Marshal(typed)
		if err == nil {
			return string(data)
		}
		return fmt.Sprint(value)
	default:
		return fmt.Sprint(value)
	}
}

func headerValue(value any) string {
	switch typed := value.(type) {
	case []string:
		return strings.Join(typed, ", ")
	case []any:
		values := make([]string, len(typed))
		for i, item := range typed {
			values[i] = queryValue(item)
		}
		return strings.Join(values, ", ")
	case map[string]any:
		data, err := json.Marshal(typed)
		if err == nil {
			return string(data)
		}
		return fmt.Sprint(value)
	default:
		return fmt.Sprint(value)
	}
}

func responseText(data []byte, contentType string) string {
	if isJSONMedia(contentType) || json.Valid(data) {
		var output bytes.Buffer
		if err := json.Indent(&output, data, "", "  "); err == nil {
			return strings.TrimRight(output.String(), "\n")
		}
	}
	return string(data)
}

func isJSONMedia(value string) bool {
	base := mediaBase(value)
	return base == "application/json" || strings.HasSuffix(base, "+json")
}

func truncate(text string, total int64) string {
	if len(text) <= maxOutput {
		return text
	}
	if int64(len(text)) > total {
		total = int64(len(text))
	}
	text = text[:maxOutput]
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + fmt.Sprintf("[truncated: %d bytes total]", total)
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}
