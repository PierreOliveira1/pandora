package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/skill"
)

type InstallOptions struct {
	MCP bool
}

type InstallReport struct {
	Client          install.Client `json:"client"`
	Mode            string         `json:"mode"`
	SkillDir        string         `json:"skill_dir"`
	SkillChanged    bool           `json:"skill_changed"`
	PermissionFile  string         `json:"permission_file,omitempty"`
	PermissionAdded bool           `json:"permission_added"`
	PermissionNote  string         `json:"permission_note,omitempty"`
	MCPConfig       string         `json:"mcp_config,omitempty"`
}

type DoctorCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Info string `json:"info,omitempty"`
	Fix  string `json:"fix,omitempty"`
}

func (c *Core) SkillDoc(ctx context.Context) (skill.Doc, error) {
	if err := ctx.Err(); err != nil {
		return skill.Doc{}, err
	}
	cfg, err := c.store.Load()
	if err != nil {
		return skill.Doc{}, err
	}

	serviceNames := sortedMapKeys(cfg.Services)
	doc := skill.Doc{Services: make([]skill.Service, 0, len(serviceNames))}
	for _, serviceName := range serviceNames {
		service := cfg.Services[serviceName]
		accounts := make([]skill.Account, 0, len(service.Accounts))
		for _, accountName := range sortedMapKeys(service.Accounts) {
			account := service.Accounts[accountName]
			accounts = append(accounts, skill.Account{
				Name:        accountName,
				Description: account.Description,
				ReadOnly:    account.ReadOnly,
			})
		}

		entry := skill.Service{
			Name:           serviceName,
			Kind:           string(service.Kind),
			Description:    service.Description,
			DefaultAccount: service.DefaultAccount,
			Accounts:       accounts,
		}
		if catalog, err := c.loadCatalog(serviceName); err == nil {
			entry.ToolsKnown = true
			entry.Instructions = catalog.Instructions
			entry.Tools = skillTools(catalog.Tools)
		}
		doc.Services = append(doc.Services, entry)
	}
	return doc, nil
}

func (c *Core) SyncSkills(ctx context.Context) ([]install.Result, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	clients, errs := integrationClients(cfg.Integrations)
	results := make([]install.Result, 0, len(clients))
	needed := make([]install.Client, 0, len(clients))
	for _, client := range clients {
		if !c.usesMCP(ctx, client) {
			needed = append(needed, client)
		}
	}
	if len(needed) == 0 {
		return results, errors.Join(errs...)
	}

	doc, err := c.SkillDoc(ctx)
	if err != nil {
		return nil, err
	}
	files, err := skill.Render(doc)
	if err != nil {
		return nil, err
	}

	for _, client := range needed {
		result, err := install.WriteSkill(ctx, c.installEnv, client, files)
		if err != nil {
			errs = append(errs, fmt.Errorf("syncing skill for %s: %w", client, err))
			continue
		}
		results = append(results, result)
	}
	return results, errors.Join(errs...)
}

func (c *Core) Install(ctx context.Context, clients []install.Client, o InstallOptions) ([]InstallReport, error) {
	if len(clients) == 0 {
		if o.MCP {
			return []InstallReport{}, errors.New("MCP mode requires a client name: use pandora install --mcp <client>")
		}
		clients = install.DetectSkillClients(c.installEnv)
	}
	clients = uniqueClients(clients)
	if len(clients) == 0 {
		return []InstallReport{}, errors.New("no AI clients found: pass one of claude-code, opencode, codex")
	}

	env := c.installEnv
	executable := env.Executable
	if needsExecutable(clients, o.MCP) {
		resolved, err := resolveExecutable(env)
		if err != nil {
			return []InstallReport{}, err
		}
		executable = resolved
		env.Executable = executable
	}

	var files map[string][]byte
	if hasSkillClient(clients, o.MCP) {
		doc, err := c.SkillDoc(ctx)
		if err != nil {
			return []InstallReport{}, err
		}
		files, err = skill.Render(doc)
		if err != nil {
			return []InstallReport{}, err
		}
	}

	reports := make([]InstallReport, 0, len(clients))
	installed := make([]string, 0, len(clients))
	var errs []error
	for _, client := range clients {
		if o.MCP || !supportsSkillClient(client) {
			report := InstallReport{Client: client, Mode: "mcp"}
			result, err := install.Install(ctx, env, client, install.Entry{
				Name:    "pandora",
				Command: executable,
				Args:    []string{"serve"},
			})
			report.MCPConfig = result.ConfigPath
			if err != nil {
				errs = append(errs, fmt.Errorf("installing MCP server for %s: %w", client, err))
				reports = append(reports, report)
				continue
			}
			if supportsSkillClient(client) {
				if has, err := install.HasSkill(env, client); err != nil {
					errs = append(errs, fmt.Errorf("checking skill of %s: %w", client, err))
				} else if has {
					if _, err := install.RemoveSkill(ctx, env, client); err != nil {
						errs = append(errs, fmt.Errorf("removing skill of %s: %w", client, err))
					}
				}
			}
			reports = append(reports, report)
			installed = append(installed, string(client))
			continue
		}

		report := InstallReport{Client: client, Mode: "skill"}
		if present, err := install.IsInstalled(ctx, env, client, "pandora"); err != nil {
			errs = append(errs, fmt.Errorf("checking MCP server of %s: %w", client, err))
		} else if present {
			if _, err := install.Uninstall(ctx, env, client, "pandora"); err != nil {
				errs = append(errs, fmt.Errorf("removing MCP server of %s: %w", client, err))
			}
		}
		written, err := install.WriteSkill(ctx, env, client, files)
		if err != nil {
			errs = append(errs, fmt.Errorf("installing skill for %s: %w", client, err))
			reports = append(reports, report)
			continue
		}
		report.SkillDir = written.ConfigPath
		report.SkillChanged = written.Changed
		permission, err := install.AllowCommand(ctx, env, client)
		switch {
		case err == nil:
			report.PermissionFile = permission.ConfigPath
			report.PermissionAdded = permission.Changed
		case errors.Is(err, install.ErrNoPermissionConfig):
			report.PermissionNote = fmt.Sprintf("allow the pandora command in %s manually", client)
		default:
			errs = append(errs, fmt.Errorf("allowing pandora in %s: %w", client, err))
		}
		reports = append(reports, report)
		installed = append(installed, string(client))
	}

	if len(installed) > 0 {
		if err := c.addIntegrations(installed); err != nil {
			errs = append(errs, err)
		}
	}
	return reports, errors.Join(errs...)
}

func (c *Core) Uninstall(ctx context.Context, clients []install.Client) ([]InstallReport, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	var errs []error
	if len(clients) == 0 {
		clients, errs = integrationClients(cfg.Integrations)
	}
	clients = uniqueClients(clients)

	reports := make([]InstallReport, 0, len(clients))
	removed := make([]string, 0, len(clients))
	for _, client := range clients {
		report := InstallReport{Client: client, Mode: "mcp"}
		if supportsSkillClient(client) {
			report.Mode = "skill"
			skillResult, err := install.RemoveSkill(ctx, c.installEnv, client)
			report.SkillDir = skillResult.ConfigPath
			report.SkillChanged = skillResult.Changed
			if err != nil {
				errs = append(errs, fmt.Errorf("removing skill from %s: %w", client, err))
			}
			permission, err := install.DisallowCommand(ctx, c.installEnv, client)
			switch {
			case err == nil:
				report.PermissionFile = permission.ConfigPath
				report.PermissionAdded = permission.Changed
			case errors.Is(err, install.ErrNoPermissionConfig):
			default:
				errs = append(errs, fmt.Errorf("removing pandora permission from %s: %w", client, err))
			}
		}
		mcpResult, err := install.Uninstall(ctx, c.installEnv, client, "pandora")
		report.MCPConfig = mcpResult.ConfigPath
		if err != nil {
			errs = append(errs, fmt.Errorf("removing MCP server from %s: %w", client, err))
		}
		if mcpResult.Changed {
			report.Mode = "mcp"
		}
		reports = append(reports, report)
		removed = append(removed, string(client))
	}

	if len(removed) > 0 {
		if err := c.removeIntegrations(removed); err != nil {
			errs = append(errs, err)
		}
	}
	return reports, errors.Join(errs...)
}

func (c *Core) Doctor(ctx context.Context, offline bool) []DoctorCheck {
	checks := make([]DoctorCheck, 0)
	configPath := c.store.Paths().ConfigFile()
	cfg, err := c.store.Load()
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name: "config",
			Info: err.Error(),
			Fix:  fmt.Sprintf("fix or remove %s: %v", configPath, err),
		})
	} else {
		checks = append(checks, DoctorCheck{Name: "config", OK: true, Info: configPath})
	}

	checks = append(checks, c.doctorSecretStore())
	checks = append(checks, c.doctorPath())
	if err == nil {
		checks = append(checks, c.doctorSkills(ctx, cfg.Integrations)...)
	}
	if !offline {
		checks = append(checks, c.doctorAccounts(ctx)...)
	}
	return checks
}

func (c *Core) addIntegrations(clients []string) error {
	return c.store.Update(func(cfg *config.Config) error {
		cfg.Integrations = mergeStrings(cfg.Integrations, clients)
		return nil
	})
}

func (c *Core) removeIntegrations(clients []string) error {
	remove := make(map[string]bool, len(clients))
	for _, client := range clients {
		remove[client] = true
	}
	return c.store.Update(func(cfg *config.Config) error {
		kept := make([]string, 0, len(cfg.Integrations))
		for _, client := range cfg.Integrations {
			if !remove[client] {
				kept = append(kept, client)
			}
		}
		cfg.Integrations = mergeStrings(kept, nil)
		return nil
	})
}

func (c *Core) doctorSecretStore() DoctorCheck {
	const key = "_doctor/probe"
	const value = "ok"
	fix := "the system keychain is not reachable; set PANDORA_SECRETS=file to use a local file"
	check := DoctorCheck{Name: "secret store"}
	fail := func(err error) DoctorCheck {
		check.Info = err.Error()
		check.Fix = fix
		return check
	}

	if err := c.secrets.Set(key, value); err != nil {
		return fail(err)
	}
	stored, err := c.secrets.Get(key)
	if err != nil {
		_ = c.secrets.Delete(key)
		return fail(err)
	}
	if stored != value {
		_ = c.secrets.Delete(key)
		return fail(errors.New("stored probe value did not match"))
	}
	if err := c.secrets.Delete(key); err != nil {
		return fail(err)
	}
	check.OK = true
	check.Info = "ok"
	return check
}

func (c *Core) doctorPath() DoctorCheck {
	check := DoctorCheck{Name: "pandora on PATH"}
	if c.installEnv.LookPath == nil {
		check.Info = "pandora not found on PATH"
		check.Fix = pandoraPathFix()
		return check
	}
	path, err := c.installEnv.LookPath("pandora")
	if err != nil {
		check.Info = "pandora not found on PATH"
		check.Fix = pandoraPathFix()
		return check
	}
	check.OK = true
	check.Info = path
	return check
}

func (c *Core) doctorSkills(ctx context.Context, integrations []string) []DoctorCheck {
	names := uniqueStrings(integrations)
	doc, docErr := c.SkillDoc(ctx)
	var files map[string][]byte
	if docErr == nil {
		files, docErr = skill.Render(doc)
	}

	checks := make([]DoctorCheck, 0, len(names))
	for _, name := range names {
		check := DoctorCheck{Name: "skill in " + name}
		client, err := install.ParseClient(name)
		if err != nil {
			check.Info = err.Error()
			check.Fix = "run pandora install"
			checks = append(checks, check)
			continue
		}
		if c.usesMCP(ctx, client) {
			check.Name = "mcp in " + name
			configPath, err := install.ConfigPath(c.installEnv, client)
			if err != nil {
				check.Info = err.Error()
				check.Fix = "run pandora install --mcp " + name
			} else {
				check.OK = true
				check.Info = configPath
			}
			checks = append(checks, check)
			continue
		}
		if docErr != nil {
			check.Info = docErr.Error()
			check.Fix = "run pandora sync"
			checks = append(checks, check)
			continue
		}
		upToDate, err := install.SkillUpToDate(ctx, c.installEnv, client, files)
		if err != nil {
			check.Info = err.Error()
			check.Fix = "run pandora sync"
			checks = append(checks, check)
			continue
		}
		if !upToDate {
			check.Info = "out of date"
			check.Fix = "run pandora sync"
			checks = append(checks, check)
			continue
		}
		check.OK = true
		check.Info = "up to date"
		checks = append(checks, check)
	}
	return checks
}

func (c *Core) doctorAccounts(ctx context.Context) []DoctorCheck {
	statuses := c.StatusAll(ctx)
	checks := make([]DoctorCheck, 0, len(statuses))
	for _, info := range statuses {
		check := DoctorCheck{Name: "account " + info.Ref}
		switch {
		case info.NeedsLogin:
			check.Info = "needs login"
			check.Fix = "run pandora auth " + info.Ref
		case !info.OK:
			check.Info = info.Error
			check.Fix = info.Error
			if check.Info == "" {
				check.Info = "connection failed"
			}
			if check.Fix == "" {
				check.Fix = "run pandora status " + info.Ref
			}
		default:
			check.OK = true
			check.Info = "ok"
		}
		checks = append(checks, check)
	}
	return checks
}

func integrationClients(names []string) ([]install.Client, []error) {
	clients := make([]install.Client, 0, len(names))
	var errs []error
	for _, name := range names {
		client, err := install.ParseClient(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		clients = append(clients, client)
	}
	return uniqueClients(clients), errs
}

func uniqueClients(clients []install.Client) []install.Client {
	seen := make(map[install.Client]bool, len(clients))
	out := make([]install.Client, 0, len(clients))
	for _, client := range clients {
		if seen[client] {
			continue
		}
		seen[client] = true
		out = append(out, client)
	}
	return out
}

func supportsSkillClient(client install.Client) bool {
	switch client {
	case install.ClaudeCode, install.OpenCode, install.Codex:
		return true
	default:
		return false
	}
}

func needsExecutable(clients []install.Client, mcp bool) bool {
	if mcp {
		return true
	}
	for _, client := range clients {
		if !supportsSkillClient(client) {
			return true
		}
	}
	return false
}

func hasSkillClient(clients []install.Client, mcp bool) bool {
	if mcp {
		return false
	}
	for _, client := range clients {
		if supportsSkillClient(client) {
			return true
		}
	}
	return false
}

func resolveExecutable(env install.Env) (string, error) {
	path := env.Executable
	if path == "" && env.LookPath != nil {
		if found, err := env.LookPath("pandora"); err == nil {
			path = found
		}
	}
	if path == "" {
		executable, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("finding the pandora executable: %w", err)
		}
		path = executable
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving the pandora executable %s: %w", path, err)
	}
	return absolute, nil
}

func (c *Core) usesMCP(ctx context.Context, client install.Client) bool {
	if supportsSkillClient(client) {
		hasSkill, err := install.HasSkill(c.installEnv, client)
		if err != nil || hasSkill {
			return false
		}
	}
	installed, err := install.IsInstalled(ctx, c.installEnv, client, "pandora")
	return err == nil && installed
}

func mergeStrings(existing, added []string) []string {
	seen := make(map[string]bool, len(existing)+len(added))
	for _, value := range existing {
		seen[value] = true
	}
	for _, value := range added {
		seen[value] = true
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(values []string) []string {
	return mergeStrings(values, nil)
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func skillTools(tools []*mcp.Tool) []skill.Tool {
	out := make([]skill.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		entry := skill.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: schemaMap(tool.InputSchema),
		}
		if tool.Annotations != nil {
			entry.ReadOnly = tool.Annotations.ReadOnlyHint
			entry.Destructive = tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func schemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func pandoraPathFix() string {
	executable, err := os.Executable()
	if err != nil || executable == "" {
		return "add the pandora binary directory to your PATH so AI clients can run pandora"
	}
	return fmt.Sprintf("add %s to your PATH so AI clients can run pandora", filepath.Dir(executable))
}
