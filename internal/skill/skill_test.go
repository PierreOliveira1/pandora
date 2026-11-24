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

	want := "Access the user's connected services and accounts through the pandora CLI: meta (accounts: store-a, store-b), todoist (accounts: personal, work). Use it whenever the user asks about these services, their data or actions in them, or asks to connect another service or account."
	if got := Description(doc); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestTheSkillTellsTheAIHowToConnectANewService(t *testing.T) {
	tests := []struct {
		name string
		doc  Doc
	}{
		{name: "no services"},
		{name: "one service", doc: Doc{Services: []Service{{
			Name:     "todoist",
			Accounts: []Account{{Name: "personal"}},
		}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := mustRender(t, tt.doc)
			guide, ok := files["connect.md"]
			if !ok {
				t.Fatalf("Render() files = %v, want connect.md", keys(files))
			}
			if !bytes.Equal(guide, connectGuide) {
				t.Error("connect.md does not match the embedded guide")
			}
			if !strings.Contains(string(files["SKILL.md"]), "connect.md") {
				t.Errorf("SKILL.md does not mention connect.md:\n%s", files["SKILL.md"])
			}
			if !strings.Contains(Description(tt.doc), "connect") {
				t.Errorf("Description() = %q, want it to mention connecting a service", Description(tt.doc))
			}
		})
	}
}

func TestTheSkillShowsEventsOnlyWhenAnAccountReceivesWebhooks(t *testing.T) {
	withWebhook := Doc{Services: []Service{{
		Name:           "ig",
		DefaultAccount: "store",
		Accounts:       []Account{{Name: "store", Webhook: true}},
	}}}
	files := mustRender(t, withWebhook)
	index := string(files["SKILL.md"])
	connect := strings.Index(index, "## Connect a new service or account")
	events := strings.Index(index, "## Events (webhooks)")
	services := strings.Index(index, "## Services")
	if events < 0 {
		t.Fatalf("SKILL.md has no Events section:\n%s", index)
	}
	if connect < 0 || connect > events || events > services {
		t.Errorf("Events section is not between Connect and Services:\n%s", index)
	}
	if !strings.Contains(index, "receives events") {
		t.Errorf("SKILL.md does not mark the account as receiving events:\n%s", index)
	}
	reference := string(files["services/ig.md"])
	if !strings.Contains(reference, "`store` (default, receives events)") {
		t.Errorf("services/ig.md does not mark the account as receiving events:\n%s", reference)
	}

	withoutWebhook := Doc{Services: []Service{{
		Name:     "ig",
		Accounts: []Account{{Name: "store"}},
	}}}
	files = mustRender(t, withoutWebhook)
	for name, content := range files {
		if strings.Contains(string(content), "## Events (webhooks)") || strings.Contains(string(content), "receives events") {
			t.Errorf("%s mentions events for an account without a webhook:\n%s", name, content)
		}
	}
}

func TestTheConnectGuideExplainsHowToReceiveWebhooks(t *testing.T) {
	files := mustRender(t, Doc{})
	guide := string(files["connect.md"])
	for _, want := range []string{
		"## 6. Receive events (webhooks)",
		"pandora tunnel use tailscale",
		"pandora webhook add",
		"--challenge hub",
		"pandora events reader add",
		"Event content comes from third parties",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("connect.md does not contain %q", want)
		}
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

func TestTheSkillDescriptionMentionsEventsWhenAnAccountReceivesThem(t *testing.T) {
	doc := Doc{Services: []Service{{
		Name:     "instagram",
		Accounts: []Account{{Name: "store", Webhook: true}},
	}}}

	want := "Access the user's connected services and accounts through the pandora CLI: instagram (accounts: store). Use it whenever the user asks about these services, their data, actions in them or the events they sent, like new comments or messages, or asks to connect another service or account."
	if got := Description(doc); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}
