package skill

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func mustRender(t *testing.T, doc Doc) map[string][]byte {
	t.Helper()
	files, err := Render(doc)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return files
}

func TestTheSkillDescriptionListsEveryConnectedServiceAndItsAccounts(t *testing.T) {
	doc := Doc{Services: []Service{
		{
			Name: "meta",
			Accounts: []Account{
				{Name: "store-b"},
				{Name: "store-a"},
			},
		},
		{
			Name:           "todoist",
			DefaultAccount: "personal",
			Accounts: []Account{
				{Name: "work"},
				{Name: "personal"},
			},
		},
	}}

	want := "Access the user's connected services and accounts through the pandora CLI: meta (accounts: store-a, store-b), todoist (accounts: personal, work). Use it whenever the user asks about these services, their data or actions in them."
	if got := Description(doc); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestALongListOfServicesKeepsTheDescriptionWithin1024Characters(t *testing.T) {
	services := make([]Service, 60)
	for i := range services {
		accounts := make([]Account, 5)
		for j := range accounts {
			accounts[j] = Account{Name: "account-" + string(rune('a'+j))}
		}
		services[i] = Service{
			Name:     "service-" + string(rune('a'+i/26)) + string(rune('a'+i%26)),
			Accounts: accounts,
		}
	}

	got := Description(Doc{Services: services})
	if len(got) > MaxDescription {
		t.Errorf("len(Description()) = %d, want at most %d", len(got), MaxDescription)
	}
	if !strings.Contains(got, "more services") {
		t.Errorf("Description() = %q, want a mention of the remaining services", got)
	}
	if !strings.Contains(got, "service-aa (5 accounts)") {
		t.Errorf("Description() = %q, want the account list replaced by a count", got)
	}
	if strings.Contains(got, "account-a") {
		t.Errorf("Description() = %q, want no individual account names", got)
	}
}

func TestEachServiceGetsAReferenceFileWithItsToolsAndArguments(t *testing.T) {
	doc := Doc{Services: []Service{{
		Name:           "todoist",
		Kind:           "mcp-http",
		Description:    "Personal tasks.",
		Instructions:   "Use labels for projects.",
		DefaultAccount: "personal",
		Accounts: []Account{
			{Name: "work", Description: "Work tasks", ReadOnly: true},
			{Name: "personal", Description: "My tasks"},
		},
		ToolsKnown: true,
		Tools: []Tool{
			{
				Name:        "add",
				Description: "Create a task.",
				Destructive: true,
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{"type": "string"},
						"done":  map[string]any{"type": "boolean"},
						"tags":  map[string]any{"type": "array"},
						"meta":  map[string]any{"type": "object"},
					},
					"required": []any{"title"},
				},
			},
			{
				Name:        "search",
				Description: "Search tasks.",
				ReadOnly:    true,
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Words to find",
							"enum":        []any{"a", "b"},
						},
						"limit":   map[string]any{"type": "integer", "description": "Maximum results"},
						"mode":    map[string]any{"type": []any{"string", "integer"}},
						"mystery": map[string]any{"description": "No declared type"},
						"nested": map[string]any{
							"type":        "object",
							"description": "Ignored nested details",
							"properties": map[string]any{
								"x": map[string]any{"type": "string"},
							},
						},
					},
					"required": []any{"query"},
				},
			},
		},
	}}}

	files := mustRender(t, doc)
	reference, ok := files["services/todoist.md"]
	if !ok {
		t.Fatalf("Render() files = %v, want services/todoist.md", keys(files))
	}
	content := string(reference)
	for _, want := range []string{
		"# todoist",
		"Personal tasks.",
		"## Notes from the service",
		"Use labels for projects.",
		"## Accounts",
		"- `personal` (default): My tasks",
		"- `work` (read-only): Work tasks",
		"## Tools",
		"### add (destructive)",
		"Create a task.",
		"### search (read-only)",
		"Search tasks.",
		"- `query` (string, required): Words to find. One of: a, b",
		"- `limit` (integer): Maximum results",
		"- `mode` (string|integer)",
		"- `mystery` (any): No declared type",
		"- `nested` (object): Ignored nested details",
		"Example: `pandora call todoist/personal add '{\"title\":\"<string>\"}'`",
		"Example: `pandora call todoist/personal search '{\"query\":\"<string>\"}'`",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("services/todoist.md does not contain %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "`x`") {
		t.Errorf("services/todoist.md renders nested properties:\n%s", content)
	}
}

func TestAServiceWhoseToolsWereNeverLoadedTellsTheAIHowToLoadThem(t *testing.T) {
	doc := Doc{Services: []Service{{
		Name:       "todoist",
		Accounts:   []Account{{Name: "personal"}},
		ToolsKnown: false,
	}}}

	files := mustRender(t, doc)
	index := string(files["SKILL.md"])
	if !strings.Contains(index, "- Tools: not loaded yet. Run pandora tools todoist to load them.") {
		t.Errorf("SKILL.md does not explain how to load the tools:\n%s", index)
	}
	reference := string(files["services/todoist.md"])
	if !strings.Contains(reference, "pandora tools todoist") {
		t.Errorf("services/todoist.md does not explain how to load the tools:\n%s", reference)
	}
}

func TestRenderingIsDeterministic(t *testing.T) {
	serviceA := Service{
		Name: "alpha",
		Accounts: []Account{
			{Name: "two"},
			{Name: "one"},
		},
		ToolsKnown: true,
		Tools: []Tool{
			{Name: "zeta", InputSchema: map[string]any{"properties": map[string]any{
				"b": map[string]any{"type": "string"},
				"a": map[string]any{"type": "integer"},
			}}},
			{Name: "beta"},
		},
	}
	serviceB := Service{Name: "beta", Accounts: []Account{{Name: "solo"}}}
	first := mustRender(t, Doc{Services: []Service{serviceA, serviceB}})
	second := mustRender(t, Doc{Services: []Service{serviceB, serviceA}})
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Render() differs when the input order changes")
	}
	again := mustRender(t, Doc{Services: []Service{serviceA, serviceB}})
	for name, content := range first {
		if !bytes.Equal(content, again[name]) {
			t.Errorf("Render() is not deterministic for %s", name)
		}
	}
}

func keys(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}

func TestTheSkillFrontmatterKeepsADescriptionWithColonsAndQuotesIntact(t *testing.T) {
	doc := Doc{Services: []Service{{Name: "todoist", Accounts: []Account{{Name: "personal"}}}}}
	files := mustRender(t, doc)

	var line string
	for _, candidate := range strings.Split(string(files["SKILL.md"]), "\n") {
		if strings.HasPrefix(candidate, "description: ") {
			line = strings.TrimPrefix(candidate, "description: ")
			break
		}
	}
	var got string
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("description %q is not a quoted scalar: %v", line, err)
	}
	if want := Description(doc); got != want {
		t.Errorf("frontmatter description = %q, want %q", got, want)
	}
}
