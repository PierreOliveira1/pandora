package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
	"github.com/PierreOliveira1/pandora/internal/source"
)

const (
	maxCapture = 2 * 1024 * 1024
	maxOutput  = 100 * 1024
)

type Source struct {
	instructions  string
	env           map[string]string
	svc           *config.Service
	secretEnv     map[string]bool
	secretHeaders map[string]string
	httpClient    *http.Client
	tools         []*mcp.Tool
	commands      map[string]*compiledTool
	httpTools     map[string]config.CommandTool
	httpGet       bool
}

type compiledTool struct {
	params   map[string]config.CommandParam
	template [][]templatePart
	timeout  int
}

type templatePart struct {
	text  string
	param string
}

type argumentValue struct {
	scalar  string
	array   []string
	isArray bool
}

func Open(ctx context.Context, spec source.Spec) (source.Source, error) {
	if spec.Svc == nil {
		return nil, fmt.Errorf("service %q has no command configuration", spec.Service)
	}
	httpClient := newHTTPClient(spec.HTTPClient, spec.Svc)
	result := &Source{
		instructions:  spec.Svc.Description,
		env:           cloneEnv(spec.Env),
		svc:           spec.Svc,
		secretEnv:     secretEnvSet(spec.Acc),
		secretHeaders: cloneEnv(spec.Headers),
		httpClient:    httpClient,
		tools:         make([]*mcp.Tool, 0, len(spec.Svc.Tools)+1),
		commands:      make(map[string]*compiledTool, len(spec.Svc.Tools)),
		httpTools:     make(map[string]config.CommandTool, len(spec.Svc.Tools)),
	}
	for _, tool := range spec.Svc.Tools {
		if tool.HTTP != nil {
			if err := httptool.Validate(tool, spec.Svc, result.secretEnv); err != nil {
				return nil, err
			}
			result.tools = append(result.tools, &mcp.Tool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: inputSchema(tool.Params),
				Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.ReadOnly || httpToolReadOnly(tool)},
			})
			result.httpTools[tool.Name] = tool
			continue
		}
		compiled, err := compileTool(tool)
		if err != nil {
			return nil, err
		}
		result.tools = append(result.tools, &mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema(tool.Params),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.ReadOnly},
		})
		result.commands[tool.Name] = compiled
	}
	if len(spec.Svc.URLs) > 0 {
		result.tools = append(result.tools, httpGetTool(spec.Svc))
		result.httpGet = true
	}
	return result, nil
}

func Register(openers source.Openers) {
	openers[config.KindCommand] = Open
	openers[config.KindHTTP] = Open
}

func (s *Source) ListTools(context.Context) ([]*mcp.Tool, error) {
	return append([]*mcp.Tool(nil), s.tools...), nil
}

func (s *Source) Instructions() string {
	return s.instructions
}

func (s *Source) Close() error {
	return nil
}

func (s *Source) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	if s.httpGet && name == httptool.GetTool {
		return s.callHTTPGet(ctx, args)
	}
	if tool, ok := s.httpTools[name]; ok {
		return s.callHTTPTool(ctx, tool, args)
	}
	tool, ok := s.commands[name]
	if !ok {
		return errorResult("unknown tool " + name), nil
	}
	values, _, failure := validateArguments(tool.params, args)
	if failure != nil {
		return failure, nil
	}

	argv := make([]string, 0, len(tool.template))
	afterDoubleDash := false
	for _, argument := range tool.template {
		literalDoubleDash := len(argument) == 1 && argument[0].param == "" && argument[0].text == "--"
		whole := len(argument) == 1 && argument[0].param != ""
		if whole {
			name := argument[0].param
			value, ok := values[name]
			if !ok {
				continue
			}
			if !afterDoubleDash {
				definition := tool.params[name]
				if definition.Type == "string" && strings.HasPrefix(value.scalar, "-") {
					return errorResult(fmt.Sprintf("argument %s cannot start with %q", name, "-")), nil
				}
				if definition.Type == "array" {
					for _, item := range value.array {
						if strings.HasPrefix(item, "-") {
							return errorResult(fmt.Sprintf("argument %s cannot start with %q", name, "-")), nil
						}
					}
				}
			}
			if value.isArray {
				argv = append(argv, value.array...)
			} else {
				argv = append(argv, value.scalar)
			}
			continue
		}
		var expanded strings.Builder
		missing := false
		for _, part := range argument {
			expanded.WriteString(part.text)
			if part.param == "" {
				continue
			}
			value, ok := values[part.param]
			if !ok {
				missing = true
				break
			}
			if value.isArray {
				expanded.WriteString(strings.Join(value.array, ","))
			} else {
				expanded.WriteString(value.scalar)
			}
		}
		if !missing {
			argv = append(argv, expanded.String())
		}
		if literalDoubleDash {
			afterDoubleDash = true
		}
	}
	if len(argv) == 0 {
		return errorResult("tool " + name + " produced an empty command"), nil
	}

	timeout := tool.timeoutSeconds()
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	process := exec.CommandContext(callCtx, argv[0], argv[1:]...)
	process.Env = buildEnvironment(s.env)
	stdout := &limitedBuffer{limit: maxCapture}
	stderr := &limitedBuffer{limit: maxCapture}
	process.Stdout = stdout
	process.Stderr = stderr
	if err := process.Run(); err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return errorResult(fmt.Sprintf("timed out after %ds", timeout)), nil
		}
		text := err.Error()
		if stderrText := strings.TrimRight(stderr.String(), "\n"); stderrText != "" {
			text += "\n" + stderrText
		}
		if stdoutText := strings.TrimRight(stdout.String(), "\n"); stdoutText != "" {
			text += "\n" + stdoutText
		}
		return errorResult(truncate(text, int64(len(text)))), nil
	}
	return textResult(truncate(stdout.String(), stdout.total)), nil
}

func validateArguments(params map[string]config.CommandParam, args map[string]any) (map[string]argumentValue, map[string]any, *mcp.CallToolResult) {
	values := map[string]argumentValue{}
	original := map[string]any{}
	names := make([]string, 0, len(params))
	for parameter := range params {
		names = append(names, parameter)
	}
	sort.Strings(names)
	for _, parameter := range names {
		definition := params[parameter]
		value, ok := args[parameter]
		if !ok || value == nil {
			if definition.Required {
				return nil, nil, errorResult("missing required argument " + parameter)
			}
			continue
		}
		normalized, err := normalizeValue(definition.Type, value)
		if err != nil {
			return nil, nil, errorResult(fmt.Sprintf("argument %s %s", parameter, err))
		}
		values[parameter] = normalized
		original[parameter] = value
	}
	return values, original, nil
}

func (t *compiledTool) timeoutSeconds() int {
	if t.timeout <= 0 {
		return 60
	}
	return t.timeout
}

func compileTool(tool config.CommandTool) (*compiledTool, error) {
	if len(tool.Command) == 0 {
		return nil, fmt.Errorf("tool %s: command is empty", tool.Name)
	}
	compiled := &compiledTool{
		params:   tool.Params,
		template: make([][]templatePart, len(tool.Command)),
		timeout:  tool.TimeoutSec,
	}
	for i, argument := range tool.Command {
		parts, err := parseTemplate(argument, tool.Name, tool.Params)
		if err != nil {
			return nil, err
		}
		compiled.template[i] = parts
	}
	return compiled, nil
}

func parseTemplate(argument, toolName string, params map[string]config.CommandParam) ([]templatePart, error) {
	var parts []templatePart
	remaining := argument
	for {
		open := strings.Index(remaining, "{{")
		if open < 0 {
			if remaining != "" || len(parts) == 0 {
				parts = append(parts, templatePart{text: remaining})
			}
			break
		}
		if open > 0 {
			parts = append(parts, templatePart{text: remaining[:open]})
		}
		after := remaining[open+2:]
		close := strings.Index(after, "}}")
		if close < 0 {
			return nil, fmt.Errorf("tool %s: unknown parameter %s in command", toolName, strings.TrimSpace(after))
		}
		parameter := after[:close]
		if _, ok := params[parameter]; !ok {
			return nil, fmt.Errorf("tool %s: unknown parameter %s in command", toolName, parameter)
		}
		parts = append(parts, templatePart{param: parameter})
		remaining = after[close+2:]
	}
	if len(parts) == 0 {
		parts = append(parts, templatePart{text: ""})
	}
	return parts, nil
}

func inputSchema(params map[string]config.CommandParam) map[string]any {
	properties := map[string]any{}
	required := make([]string, 0)
	for name, parameter := range params {
		property := map[string]any{"type": parameter.Type}
		if parameter.Type == "array" {
			property["items"] = map[string]any{"type": "string"}
		}
		if parameter.Description != "" {
			property["description"] = parameter.Description
		}
		properties[name] = property
		if parameter.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func normalizeValue(valueType string, value any) (argumentValue, error) {
	switch valueType {
	case "string":
		text, ok := value.(string)
		if !ok {
			return argumentValue{}, errors.New("must be a string")
		}
		return argumentValue{scalar: text}, nil
	case "number":
		if !isNumber(value) {
			return argumentValue{}, errors.New("must be a number")
		}
		return argumentValue{scalar: numberString(value)}, nil
	case "integer":
		if !isInteger(value) {
			return argumentValue{}, errors.New("must be an integer")
		}
		return argumentValue{scalar: numberString(value)}, nil
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return argumentValue{}, errors.New("must be a boolean")
		}
		return argumentValue{scalar: strconv.FormatBool(boolean)}, nil
	case "array":
		items, err := stringArray(value)
		if err != nil {
			return argumentValue{}, err
		}
		return argumentValue{array: items, isArray: true}, nil
	default:
		return argumentValue{}, fmt.Errorf("has unsupported type %s", valueType)
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func isInteger(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return float32(int64(typed)) == typed
	case float64:
		return float64(int64(typed)) == typed
	case json.Number:
		_, err := typed.Int64()
		return err == nil
	default:
		return false
	}
}

func numberString(value any) string {
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return fmt.Sprint(value)
}

func stringArray(value any) ([]string, error) {
	var source []any
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		source = typed
	default:
		return nil, errors.New("must be an array of strings")
	}
	items := make([]string, len(source))
	for i, item := range source {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("must be an array of strings")
		}
		items[i] = text
	}
	return items, nil
}

func buildEnvironment(extra map[string]string) []string {
	values := map[string]string{}
	order := make([]string, 0, len(extra))
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok := values[name]; !ok {
			order = append(order, name)
		}
		values[name] = value
	}
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ok := values[name]; !ok {
			order = append(order, name)
		}
		values[name] = extra[name]
	}
	env := make([]string, 0, len(order))
	for _, name := range order {
		env = append(env, name+"="+values[name])
	}
	return env
}

func cloneEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	cloned := make(map[string]string, len(env))
	for name, value := range env {
		cloned[name] = value
	}
	return cloned
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
	total  int64
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	b.total += int64(len(data))
	if b.buffer.Len() < b.limit {
		remaining := b.limit - b.buffer.Len()
		if remaining > len(data) {
			remaining = len(data)
		}
		_, _ = b.buffer.Write(data[:remaining])
	}
	return len(data), nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
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
