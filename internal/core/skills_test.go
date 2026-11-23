package core_test

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/skill"
)

func TestInstallingWritesASkillThatListsTheConnectedServicesAndAllowsThePandoraCommand(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.home, ".claude"), 0o755); err != nil {
		t.Fatalf("creating claude directory: %v", err)
	}
	f.addFake(t, "fake", "a", "ana")

	reports, err := f.core.Install(context.Background(), nil, core.InstallOptions{})
	if err != nil {
		t.Fatalf("installing skill: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	report := reports[0]
	if report.Client != install.ClaudeCode {
		t.Fatalf("client = %q, want %q", report.Client, install.ClaudeCode)
	}
	if !report.SkillChanged {
		t.Fatal("SkillChanged = false, want true")
	}
	settingsPath := filepath.Join(f.home, ".claude", "settings.json")
	if report.PermissionFile != settingsPath {
		t.Fatalf("PermissionFile = %q, want %q", report.PermissionFile, settingsPath)
	}
	if !report.PermissionAdded {
		t.Fatal("PermissionAdded = false, want true")
	}

	skillPath := filepath.Join(f.home, ".claude", "skills", "pandora", "SKILL.md")
	skillData, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("reading skill: %v", err)
	}
	if !strings.Contains(string(skillData), "fake (accounts: a)") {
		t.Fatalf("SKILL.md = %q, want the fake service and account", skillData)
	}
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if !strings.Contains(string(settings), "Bash(pandora:*)") {
		t.Fatalf("settings.json = %q, want the pandora permission", settings)
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if !slices.Contains(cfg.Integrations, string(install.ClaudeCode)) {
		t.Fatalf("Integrations = %v, want claude-code", cfg.Integrations)
	}
}

func TestTheSkillShowsTheToolsOfAServiceAfterTheyAreLoaded(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "ana")
	ctx := context.Background()

	before := renderSkill(t, f.core)
	if !strings.Contains(string(before["SKILL.md"]), "not loaded yet") {
		t.Fatalf("SKILL.md = %q, want tools not loaded", before["SKILL.md"])
	}

	if _, err := f.core.Tools(ctx, "fake", true); err != nil {
		t.Fatalf("loading tools: %v", err)
	}
	after := renderSkill(t, f.core)
	serviceDoc, ok := after["services/fake.md"]
	if !ok {
		names := make([]string, 0, len(after))
		for name := range after {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Fatalf("skill files = %v, want services/fake.md", names)
	}
	if !strings.Contains(string(serviceDoc), "whoami") {
		t.Fatalf("services/fake.md = %q, want the whoami tool", serviceDoc)
	}
}

func TestDoctorPointsToSyncWhenASkillIsOutOfDate(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.home, ".claude"), 0o755); err != nil {
		t.Fatalf("creating claude directory: %v", err)
	}
	f.addFake(t, "fake", "a", "ana")
	if _, err := f.core.Install(context.Background(), nil, core.InstallOptions{}); err != nil {
		t.Fatalf("installing skill: %v", err)
	}

	skillPath := filepath.Join(f.home, ".claude", "skills", "pandora", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("hand edited\n"), 0o644); err != nil {
		t.Fatalf("editing skill: %v", err)
	}

	checks := f.core.Doctor(context.Background(), true)
	var skillCheck *core.DoctorCheck
	for i := range checks {
		if checks[i].Name == "skill in claude-code" {
			skillCheck = &checks[i]
			break
		}
	}
	if skillCheck == nil {
		t.Fatalf("checks = %+v, want a skill in claude-code check", checks)
	}
	if skillCheck.OK {
		t.Fatal("skill check OK = true, want false")
	}
	if skillCheck.Fix != "run pandora sync" {
		t.Fatalf("fix = %q, want %q", skillCheck.Fix, "run pandora sync")
	}
}

func TestAClientWithoutSkillsIsInstalledAsAnMCPServer(t *testing.T) {
	f := newFixture(t)
	app := newMCPCore(t, f)
	f.addFake(t, "fake", "a", "ana")

	reports, err := app.Install(context.Background(), []install.Client{install.Cursor}, core.InstallOptions{})
	if err != nil {
		t.Fatalf("installing MCP server: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	report := reports[0]
	mcpPath := filepath.Join(f.home, ".cursor", "mcp.json")
	if report.Client != install.Cursor || report.Mode != "mcp" || report.MCPConfig != mcpPath {
		t.Fatalf("report = %+v, want cursor in MCP mode at %s", report, mcpPath)
	}
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("reading MCP config: %v", err)
	}
	var document struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decoding MCP config: %v", err)
	}
	entry, ok := document.MCPServers["pandora"]
	if !ok {
		t.Fatalf("MCP config = %s, want a pandora entry", data)
	}
	if entry.Command != "/opt/pandora" || !slices.Equal(entry.Args, []string{"serve"}) {
		t.Fatalf("MCP entry = %+v, want pandora serve", entry)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".cursor", "skills", "pandora")); !os.IsNotExist(err) {
		t.Fatalf("Stat(skill) error = %v, want no skill", err)
	}
}

func TestSwitchingAClientToMCPModeRemovesItsSkill(t *testing.T) {
	f := newFixture(t)
	app := newMCPCore(t, f)
	if err := os.MkdirAll(filepath.Join(f.home, ".claude"), 0o755); err != nil {
		t.Fatalf("creating claude directory: %v", err)
	}
	f.addFake(t, "fake", "a", "ana")
	ctx := context.Background()

	if _, err := app.Install(ctx, []install.Client{install.ClaudeCode}, core.InstallOptions{}); err != nil {
		t.Fatalf("installing skill: %v", err)
	}
	skillDir, err := install.SkillDir(f.installEnv, install.ClaudeCode)
	if err != nil {
		t.Fatalf("finding skill directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("skill was not installed: %v", err)
	}

	reports, err := app.Install(ctx, []install.Client{install.ClaudeCode}, core.InstallOptions{MCP: true})
	if err != nil {
		t.Fatalf("switching to MCP: %v", err)
	}
	if len(reports) != 1 || reports[0].Mode != "mcp" || reports[0].MCPConfig != filepath.Join(f.home, ".claude.json") {
		t.Fatalf("reports = %+v, want claude-code in MCP mode", reports)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("Stat(skill) error = %v, want the skill removed", err)
	}
	installed, err := install.IsInstalled(ctx, f.installEnv, install.ClaudeCode, "pandora")
	if err != nil {
		t.Fatalf("checking MCP entry: %v", err)
	}
	if !installed {
		t.Fatal("MCP entry was not installed")
	}

	if _, err := app.Uninstall(ctx, []install.Client{install.ClaudeCode}); err != nil {
		t.Fatalf("uninstalling: %v", err)
	}
	installed, err = install.IsInstalled(ctx, f.installEnv, install.ClaudeCode, "pandora")
	if err != nil {
		t.Fatalf("checking MCP entry after uninstall: %v", err)
	}
	if installed {
		t.Fatal("MCP entry remained after uninstall")
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("Stat(skill) error = %v, want no skill after uninstall", err)
	}
	settings, err := os.ReadFile(filepath.Join(f.home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if strings.Contains(string(settings), "Bash(pandora:*)") {
		t.Fatal("the pandora permission remained after uninstall")
	}
}

func newMCPCore(t *testing.T, f *fixture) *core.Core {
	t.Helper()
	env := f.installEnv
	env.Executable = "/opt/pandora"
	app := core.New(core.Options{
		Store:      f.store,
		Secrets:    f.secrets,
		Openers:    f.openers,
		Version:    "test",
		InstallEnv: env,
	})
	t.Cleanup(func() {
		_ = app.Close()
	})
	return app
}

func renderSkill(t *testing.T, app *core.Core) map[string][]byte {
	t.Helper()
	doc, err := app.SkillDoc(context.Background())
	if err != nil {
		t.Fatalf("building skill doc: %v", err)
	}
	files, err := skill.Render(doc)
	if err != nil {
		t.Fatalf("rendering skill: %v", err)
	}
	return files
}

func TestTheAIClientsListShowsHowEachClientReachesPandora(t *testing.T) {
	f := newFixture(t)
	app := newMCPCore(t, f)
	f.addFake(t, "fake", "a", "ana")
	if err := os.MkdirAll(filepath.Join(f.home, ".claude"), 0o700); err != nil {
		t.Fatalf("creating claude dir: %v", err)
	}
	if _, err := app.Install(context.Background(), []install.Client{install.ClaudeCode, install.Cursor}, core.InstallOptions{}); err != nil {
		t.Fatalf("installing: %v", err)
	}

	modes := map[install.Client]string{}
	for _, view := range app.Integrations(context.Background()) {
		modes[view.Client] = view.Mode
	}
	want := map[install.Client]string{
		install.ClaudeCode:    "skill",
		install.Cursor:        "mcp",
		install.OpenCode:      "",
		install.Codex:         "",
		install.ClaudeDesktop: "",
	}
	if !maps.Equal(modes, want) {
		t.Fatalf("modes = %v, want %v", modes, want)
	}
}
