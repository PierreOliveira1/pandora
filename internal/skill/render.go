package skill

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed connect.md
var connectGuide []byte

func Render(doc Doc) (map[string][]byte, error) {
	services := normalizedServices(doc.Services)
	files := make(map[string][]byte, len(services)+2)
	files["SKILL.md"] = renderSkillMD(Description(doc), services)
	files["connect.md"] = append([]byte(nil), connectGuide...)
	for _, service := range services {
		if service.Name == "" || service.Name == "." || service.Name == ".." || strings.ContainsAny(service.Name, `/\`) {
			return nil, fmt.Errorf("invalid service name %q", service.Name)
		}
		path := "services/" + service.Name + ".md"
		files[path] = renderService(service)
	}
	return files, nil
}

func renderSkillMD(description string, services []Service) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: pandora\n")
	b.WriteString("description: ")
	b.WriteString(yamlString(description))
	b.WriteString("\n---\n\n")
	b.WriteString("# Pandora\n\n")
	b.WriteString("Pandora connects you to the user's services and accounts. Use the `pandora` command in the terminal. Never guess tool names or arguments: look them up first.\n\n")
	b.WriteString("## How to work\n\n")
	b.WriteString("1. Pick the service and account from the list below. When the user does not name an account and the service has more than one, use the default account only if the request clearly fits it; otherwise ask the user.\n")
	b.WriteString("2. Find the tool: read `services/<service>.md` in this skill, or run `pandora search \"<words>\"`.\n")
	b.WriteString("3. Check its arguments: `pandora describe <service> <tool>`.\n")
	b.WriteString("4. Run it: `pandora call <service>/<account> <tool> '{\"arg\": \"value\"}'`. For long JSON or JSON with quotes, pass it on stdin:\n\n")
	b.WriteString("```\n")
	b.WriteString("pandora call <service>/<account> <tool> - <<'JSON'\n")
	b.WriteString("{\"arg\": \"value\"}\n")
	b.WriteString("JSON\n")
	b.WriteString("```\n\n")
	b.WriteString("5. Add `--json` to any command for machine-readable output, and pipe it to `jq` when you only need part of it.\n\n")
	b.WriteString("If a command says an account needs login, ask the user to run `pandora auth <service>/<account>` (it opens the browser). Never try to log in yourself.\n")
	b.WriteString("Tools marked read-only only read data. Confirm with the user before running tools that create, change or delete data, spend money or send messages.\n\n")
	b.WriteString("## Connect a new service or account\n\nWhen the user asks to connect a service, an API or another account, read `connect.md` in this skill before you run anything.\n\n")
	b.WriteString("## Services\n\n")
	if len(services) == 0 {
		b.WriteString("No services are connected yet. To connect one, read `connect.md`.\n")
		return []byte(b.String())
	}
	for i, service := range services {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("### " + service.Name + "\n")
		if description := strings.TrimSpace(service.Description); description != "" {
			b.WriteString(description + "\n")
		}
		b.WriteString("- Accounts: " + accountsLine(service) + "\n")
		b.WriteString(toolsLine(service) + "\n")
	}
	return []byte(b.String())
}

func renderService(service Service) []byte {
	sections := []string{"# " + service.Name}
	if description := strings.TrimSpace(service.Description); description != "" {
		sections = append(sections, description)
	}
	if instructions := strings.TrimSpace(service.Instructions); instructions != "" {
		sections = append(sections, "## Notes from the service\n\n"+instructions)
	}
	sections = append(sections, "## Accounts\n"+accountsList(service))
	sections = append(sections, "## Tools\n\n"+toolsSection(service))
	return []byte(strings.Join(sections, "\n\n") + "\n")
}

func accountsLine(service Service) string {
	if len(service.Accounts) == 0 {
		return "none"
	}
	entries := make([]string, len(service.Accounts))
	for i, account := range service.Accounts {
		entries[i] = accountEntry(service, account)
	}
	return strings.Join(entries, "; ")
}

func accountsList(service Service) string {
	if len(service.Accounts) == 0 {
		return "No accounts."
	}
	entries := make([]string, len(service.Accounts))
	for i, account := range service.Accounts {
		entries[i] = "- " + accountEntry(service, account)
	}
	return strings.Join(entries, "\n")
}

func accountEntry(service Service, account Account) string {
	entry := "`" + account.Name + "`"
	flags := make([]string, 0, 2)
	if account.Name == service.DefaultAccount {
		flags = append(flags, "default")
	}
	if account.ReadOnly {
		flags = append(flags, "read-only")
	}
	if len(flags) > 0 {
		entry += " (" + strings.Join(flags, ", ") + ")"
	}
	if description := strings.TrimSpace(account.Description); description != "" {
		entry += ": " + description
	}
	return entry
}

func toolsLine(service Service) string {
	if !service.ToolsKnown {
		return "- Tools: not loaded yet. Run pandora tools " + service.Name + " to load them."
	}
	if len(service.Tools) == 0 {
		return "- Tools (0): none"
	}
	names := make([]string, len(service.Tools))
	for i, tool := range service.Tools {
		names[i] = tool.Name
	}
	list := strings.Join(names, ", ")
	if len(names) > 15 {
		list = strings.Join(names[:15], ", ") + ", ..."
	}
	return fmt.Sprintf("- Tools (%d): %s. Full list: `services/%s.md`", len(service.Tools), list, service.Name)
}

func yamlString(value string) string {
	var out strings.Builder
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return `""`
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func toolsSection(service Service) string {
	if !service.ToolsKnown {
		return "Tools have not been loaded yet. Run pandora tools " + service.Name + " to load them."
	}
	if len(service.Tools) == 0 {
		return "This service has no tools."
	}
	tools := make([]string, len(service.Tools))
	for i, tool := range service.Tools {
		tools[i] = renderTool(service, tool)
	}
	return strings.Join(tools, "\n\n")
}

func renderTool(service Service, tool Tool) string {
	header := "### " + tool.Name
	flags := make([]string, 0, 2)
	if tool.ReadOnly {
		flags = append(flags, "read-only")
	}
	if tool.Destructive {
		flags = append(flags, "destructive")
	}
	if len(flags) > 0 {
		header += " (" + strings.Join(flags, ", ") + ")"
	}
	if description := strings.TrimSpace(tool.Description); description != "" {
		header += "\n" + description
	}
	sections := []string{
		header,
		"Arguments:\n" + argumentsList(tool.InputSchema),
		"Example: `" + exampleCall(service, tool) + "`",
	}
	return strings.Join(sections, "\n\n")
}

func argumentsList(schema map[string]any) string {
	properties, _ := schema["properties"].(map[string]any)
	required := requiredSet(schema)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "No arguments."
	}
	lines := make([]string, len(names))
	for i, name := range names {
		property, _ := properties[name].(map[string]any)
		lines[i] = argumentLine(name, property, required[name])
	}
	return strings.Join(lines, "\n")
}

func argumentLine(name string, property map[string]any, required bool) string {
	attributes := schemaType(property)
	if required {
		attributes += ", required"
	}
	line := "- `" + name + "` (" + attributes + ")"
	description := ""
	if property != nil {
		description, _ = property["description"].(string)
	}
	description = strings.TrimSpace(description)
	if description != "" {
		line += ": " + description
	}
	enum := enumValues(property)
	if len(enum) > 0 {
		if description == "" {
			line += ": One of: " + strings.Join(enum, ", ")
		} else {
			if !strings.HasSuffix(description, ".") {
				line += "."
			}
			line += " One of: " + strings.Join(enum, ", ")
		}
	}
	return line
}

func schemaType(property map[string]any) string {
	types := schemaTypes(property)
	if len(types) == 0 {
		return "any"
	}
	return strings.Join(types, "|")
}

func schemaTypes(property map[string]any) []string {
	if property == nil {
		return nil
	}
	switch value := property["type"].(type) {
	case string:
		if value == "" {
			return nil
		}
		return []string{value}
	case []string:
		return value
	case []any:
		types := make([]string, 0, len(value))
		for _, item := range value {
			if name, ok := item.(string); ok && name != "" {
				types = append(types, name)
			}
		}
		return types
	default:
		return nil
	}
}

func enumValues(property map[string]any) []string {
	if property == nil {
		return nil
	}
	var values []string
	switch enum := property["enum"].(type) {
	case []string:
		values = append(values, enum...)
	case []any:
		for _, item := range enum {
			values = append(values, fmt.Sprint(item))
		}
	}
	return values
}

func requiredSet(schema map[string]any) map[string]bool {
	required := map[string]bool{}
	switch names := schema["required"].(type) {
	case []string:
		for _, name := range names {
			required[name] = true
		}
	case []any:
		for _, item := range names {
			if name, ok := item.(string); ok {
				required[name] = true
			}
		}
	}
	return required
}

func exampleCall(service Service, tool Tool) string {
	account := service.DefaultAccount
	if account == "" && len(service.Accounts) > 0 {
		account = service.Accounts[0].Name
	}
	if account == "" {
		account = "default"
	}
	return "pandora call " + service.Name + "/" + account + " " + tool.Name + " '" + exampleArguments(tool.InputSchema) + "'"
}

func exampleArguments(schema map[string]any) string {
	properties, _ := schema["properties"].(map[string]any)
	required := requiredSet(schema)
	names := make([]string, 0, len(required))
	for name := range required {
		names = append(names, name)
	}
	sort.Strings(names)
	arguments := make(map[string]any, len(names))
	for _, name := range names {
		property, _ := properties[name].(map[string]any)
		arguments[name] = exampleValue(property)
	}
	var out strings.Builder
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(arguments); err != nil {
		return "{}"
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func exampleValue(property map[string]any) any {
	types := schemaTypes(property)
	if len(types) == 0 {
		return "<string>"
	}
	switch types[0] {
	case "integer", "number":
		return 0
	case "boolean":
		return true
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default:
		return "<string>"
	}
}
