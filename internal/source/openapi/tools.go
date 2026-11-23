package openapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxToolNameLength = 64

type rawParameter struct {
	name     string
	in       string
	required bool
	schema   map[string]any
}

func buildTools(doc *loadedSpec) ([]*mcp.Tool, map[string]*operation, string) {
	paths, _ := doc.root["paths"].(map[string]any)
	tools := make([]*mcp.Tool, 0)
	operations := map[string]*operation{}
	usedNames := map[string]bool{}
	methods := []string{"get", "put", "post", "delete", "options", "head", "patch"}
	for _, path := range sortedMapKeys(paths) {
		pathItem, _ := paths[path].(map[string]any)
		if pathItem == nil {
			continue
		}
		pathParameters := parameterSlice(pathItem["parameters"])
		for _, method := range methods {
			item, _ := pathItem[method].(map[string]any)
			if item == nil {
				continue
			}
			tool, operation := buildTool(doc.version, method, path, item, pathParameters, usedNames)
			tools = append(tools, tool)
			operations[tool.Name] = operation
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools, operations, instructions(doc.root)
}

func buildTool(version int, method, path string, item map[string]any, pathParameters []any, usedNames map[string]bool) (*mcp.Tool, *operation) {
	parameters := collectParameters(version, pathParameters, parameterSlice(item["parameters"]))
	body, form, bodyRequired := requestBody(version, item)
	properties := map[string]any{}
	required := map[string]bool{}
	usedProperties := map[string]bool{}
	if body != nil {
		usedProperties["body"] = true
	}
	operation := &operation{
		method:  method,
		path:    path,
		hasBody: body != nil,
		form:    form,
	}
	pathProperties := map[string]string{}
	for _, parameter := range parameters {
		property := parameter.name
		if usedProperties[property] {
			property = parameter.in + "_" + parameter.name
		}
		property = uniqueProperty(property, usedProperties)
		properties[property] = parameter.schema
		if parameter.required {
			required[property] = true
		}
		switch parameter.in {
		case "path":
			pathProperties[parameter.name] = property
		case "query":
			operation.query = append(operation.query, parameterRef{property: property, name: parameter.name})
		case "header":
			operation.headers = append(operation.headers, parameterRef{property: property, name: parameter.name})
		}
	}
	for _, placeholder := range pathPlaceholders(path) {
		property := placeholder
		if mapped, ok := pathProperties[placeholder]; ok {
			property = mapped
		}
		operation.pathParams = append(operation.pathParams, parameterRef{property: property, name: placeholder})
	}
	if body != nil {
		properties["body"] = body
		if bodyRequired {
			required["body"] = true
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if names := sortedSet(required); len(names) > 0 {
		schema["required"] = names
	}
	name := toolName(item, method, path, usedNames)
	annotations := &mcp.ToolAnnotations{
		ReadOnlyHint: method == "get" || method == "head" || method == "options",
	}
	if method == "delete" {
		destroy := true
		annotations.DestructiveHint = &destroy
	}
	return &mcp.Tool{
		Name:        name,
		Description: toolDescription(item, method, path),
		InputSchema: schema,
		Annotations: annotations,
	}, operation
}

func collectParameters(version int, pathValues, operationValues []any) []rawParameter {
	var out []rawParameter
	index := map[string]int{}
	add := func(values []any) {
		for _, value := range values {
			item, _ := value.(map[string]any)
			if item == nil {
				continue
			}
			name, _ := item["name"].(string)
			in, _ := item["in"].(string)
			if name == "" || in == "" {
				continue
			}
			if in != "path" && in != "query" && in != "header" {
				continue
			}
			if version == 2 && (in == "body" || in == "formData") {
				continue
			}
			if in == "header" && ignoredHeader(name) {
				continue
			}
			var schema map[string]any
			if version == 2 {
				schema = swaggerSchema(item)
			} else {
				schema = schemaWithDescription(mapValue(item["schema"]), asString(item["description"]))
			}
			parameter := rawParameter{
				name:     name,
				in:       in,
				required: in == "path" || boolValue(item, "required"),
				schema:   schema,
			}
			key := in + "\x00" + name
			if position, ok := index[key]; ok {
				out[position] = parameter
				continue
			}
			index[key] = len(out)
			out = append(out, parameter)
		}
	}
	add(pathValues)
	add(operationValues)
	return out
}

func swaggerSchema(item map[string]any) map[string]any {
	schema := map[string]any{}
	for _, key := range []string{"type", "format", "items", "enum"} {
		if value, ok := item[key]; ok {
			schema[key] = value
		}
	}
	if description := asString(item["description"]); description != "" {
		schema["description"] = description
	}
	return schema
}

func schemaWithDescription(schema map[string]any, description string) map[string]any {
	out := make(map[string]any, len(schema)+1)
	for key, value := range schema {
		out[key] = value
	}
	if description != "" {
		out["description"] = description
	}
	return out
}

func requestBody(version int, item map[string]any) (map[string]any, bool, bool) {
	if version == 2 {
		return swaggerRequestBody(item)
	}
	requestBody, _ := item["requestBody"].(map[string]any)
	if requestBody == nil {
		return nil, false, false
	}
	content, _ := requestBody["content"].(map[string]any)
	if content == nil {
		return nil, false, false
	}
	if media, ok := findMedia(content, func(base string) bool {
		return base == "application/json" || strings.HasSuffix(base, "+json")
	}); ok {
		schema := mapValue(media["schema"])
		if schema == nil {
			schema = map[string]any{}
		}
		return schema, false, boolValue(requestBody, "required")
	}
	if _, ok := findMedia(content, func(base string) bool {
		return base == "application/x-www-form-urlencoded" || base == "multipart/form-data"
	}); ok {
		return map[string]any{"type": "object", "description": "Form fields"}, true, boolValue(requestBody, "required")
	}
	return nil, false, false
}

func swaggerRequestBody(item map[string]any) (map[string]any, bool, bool) {
	parameters := parameterSlice(item["parameters"])
	for _, value := range parameters {
		parameter, _ := value.(map[string]any)
		if parameter == nil || asString(parameter["in"]) != "body" {
			continue
		}
		schema := mapValue(parameter["schema"])
		if schema == nil {
			schema = map[string]any{}
		}
		return schema, false, boolValue(parameter, "required")
	}
	properties := map[string]any{}
	required := map[string]bool{}
	for _, value := range parameters {
		parameter, _ := value.(map[string]any)
		if parameter == nil || asString(parameter["in"]) != "formData" {
			continue
		}
		name := asString(parameter["name"])
		if name == "" {
			continue
		}
		properties[name] = swaggerSchema(parameter)
		if boolValue(parameter, "required") {
			required[name] = true
		}
	}
	if len(properties) == 0 {
		return nil, false, false
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if names := sortedSet(required); len(names) > 0 {
		schema["required"] = names
	}
	return schema, true, len(required) > 0
}

func findMedia(content map[string]any, match func(base string) bool) (map[string]any, bool) {
	for _, key := range sortedMapKeys(content) {
		if !match(mediaBase(key)) {
			continue
		}
		media, _ := content[key].(map[string]any)
		return media, true
	}
	return nil, false
}

func mediaBase(value string) string {
	if index := strings.IndexByte(value, ';'); index >= 0 {
		value = value[:index]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func toolName(item map[string]any, method, path string, used map[string]bool) string {
	name := sanitizeName(asString(item["operationId"]))
	if name == "" {
		name = sanitizeName(method + "_" + path)
	}
	if name == "" {
		name = method
	}
	if !used[name] {
		used[name] = true
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", name, suffix)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

func sanitizeName(value string) string {
	var first []byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if valid {
			first = append(first, c)
			continue
		}
		if len(first) == 0 || first[len(first)-1] != '_' {
			first = append(first, '_')
		}
	}
	var collapsed []byte
	for _, c := range first {
		if c == '_' && len(collapsed) > 0 && collapsed[len(collapsed)-1] == '_' {
			continue
		}
		collapsed = append(collapsed, c)
	}
	result := strings.Trim(string(collapsed), "_")
	if len(result) > maxToolNameLength {
		result = result[:maxToolNameLength]
	}
	return result
}

func uniqueProperty(name string, used map[string]bool) string {
	if !used[name] {
		used[name] = true
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", name, suffix)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

func toolDescription(item map[string]any, method, path string) string {
	parts := make([]string, 0, 3)
	if summary := strings.TrimSpace(asString(item["summary"])); summary != "" {
		parts = append(parts, summary)
	}
	if description := strings.TrimSpace(asString(item["description"])); description != "" {
		parts = append(parts, description)
	}
	parts = append(parts, "HTTP "+strings.ToUpper(method)+" "+path)
	return strings.Join(parts, "\n\n")
}

func instructions(root map[string]any) string {
	info, _ := root["info"].(map[string]any)
	parts := make([]string, 0, 2)
	if title := strings.TrimSpace(asString(info["title"])); title != "" {
		parts = append(parts, title)
	}
	if description := strings.TrimSpace(asString(info["description"])); description != "" {
		parts = append(parts, description)
	}
	return strings.Join(parts, "\n\n")
}

func pathPlaceholders(path string) []string {
	var names []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			return names
		}
		rest := path[start+1:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return names
		}
		name := rest[:end]
		if name != "" {
			names = append(names, name)
		}
		path = rest[end+1:]
	}
}

func ignoredHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "content-type", "accept":
		return true
	default:
		return false
	}
}

func parameterSlice(value any) []any {
	values, _ := value.([]any)
	return values
}

func mapValue(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

func boolValue(object map[string]any, key string) bool {
	value, _ := object[key].(bool)
	return value
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
