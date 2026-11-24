package core

import (
	"context"
	"sort"

	"github.com/PierreOliveira1/pandora/internal/config"
)

type AccountView struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Default       bool              `json:"default"`
	ReadOnly      bool              `json:"readonly"`
	Allow         []string          `json:"allow"`
	Deny          []string          `json:"deny"`
	Env           map[string]string `json:"env,omitempty"`
	SecretEnv     []string          `json:"secret_env,omitempty"`
	SecretHeaders []string          `json:"secret_headers,omitempty"`
}

type ServiceView struct {
	Name           string               `json:"name"`
	Kind           config.Kind          `json:"kind"`
	Description    string               `json:"description,omitempty"`
	Auth           config.AuthMode      `json:"auth"`
	Expose         config.ExposeMode    `json:"expose"`
	URL            string               `json:"url,omitempty"`
	Command        []string             `json:"command,omitempty"`
	Spec           string               `json:"spec,omitempty"`
	URLs           map[string]string    `json:"urls,omitempty"`
	Headers        map[string]string    `json:"headers,omitempty"`
	DefaultAccount string               `json:"default_account,omitempty"`
	ToolsKnown     bool                 `json:"tools_known"`
	ToolCount      int                  `json:"tool_count"`
	CommandTools   []config.CommandTool `json:"command_tools,omitempty"`
	Accounts       []AccountView        `json:"accounts"`
}

type Overview struct {
	Version      string        `json:"version"`
	Services     []ServiceView `json:"services"`
	Integrations []string      `json:"integrations"`
}

func (c *Core) Overview(ctx context.Context) (Overview, error) {
	if err := ctx.Err(); err != nil {
		return Overview{}, err
	}
	cfg, err := c.store.Load()
	if err != nil {
		return Overview{}, err
	}

	serviceNames := sortedMapKeys(cfg.Services)
	services := make([]ServiceView, 0, len(serviceNames))
	for _, serviceName := range serviceNames {
		service := cfg.Services[serviceName]
		accounts := make([]AccountView, 0, len(service.Accounts))
		for _, accountName := range sortedMapKeys(service.Accounts) {
			account := service.Accounts[accountName]
			accounts = append(accounts, AccountView{
				Name:          accountName,
				Description:   account.Description,
				Default:       accountName == service.DefaultAccount,
				ReadOnly:      account.ReadOnly,
				Allow:         sortedRules(account.Allow),
				Deny:          sortedRules(account.Deny),
				Env:           copyStringMap(account.Env),
				SecretEnv:     sortedNames(account.SecretEnv),
				SecretHeaders: sortedNames(account.SecretHeaders),
			})
		}

		command := append([]string(nil), service.Command...)
		if command == nil {
			command = []string{}
		}
		commandTools := append([]config.CommandTool(nil), service.Tools...)
		if commandTools == nil {
			commandTools = []config.CommandTool{}
		}
		sort.Slice(commandTools, func(i, j int) bool {
			return commandTools[i].Name < commandTools[j].Name
		})

		entry := ServiceView{
			Name:           serviceName,
			Kind:           service.Kind,
			Description:    service.Description,
			Auth:           service.Auth,
			Expose:         service.Expose,
			URL:            service.URL,
			Command:        command,
			Spec:           service.Spec,
			URLs:           copyStringMap(service.URLs),
			Headers:        copyStringMap(service.Headers),
			DefaultAccount: service.DefaultAccount,
			CommandTools:   commandTools,
			Accounts:       accounts,
		}
		if entry.Expose == "" {
			entry.Expose = config.ExposeCompact
		}
		if catalog, err := c.loadCatalog(serviceName); err == nil {
			entry.ToolsKnown = true
			entry.ToolCount = len(catalog.Tools)
		}
		services = append(services, entry)
	}

	integrations := append([]string(nil), cfg.Integrations...)
	if integrations == nil {
		integrations = []string{}
	}
	sort.Strings(integrations)

	return Overview{
		Version:      c.version,
		Services:     services,
		Integrations: integrations,
	}, nil
}

func sortedRules(rules []string) []string {
	out := append([]string(nil), rules...)
	if out == nil {
		return []string{}
	}
	sort.Strings(out)
	return out
}

func copyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

func sortedNames(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
