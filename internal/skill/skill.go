package skill

import (
	"fmt"
	"sort"
	"strings"
)

type Doc struct {
	Services []Service
}

type Service struct {
	Name           string
	Kind           string
	Description    string
	Instructions   string
	DefaultAccount string
	Accounts       []Account
	ToolsKnown     bool
	Tools          []Tool
}

type Account struct {
	Name        string
	Description string
	ReadOnly    bool
	Webhook     bool
}

type Tool struct {
	Name        string
	Description string
	ReadOnly    bool
	Destructive bool
	InputSchema map[string]any
}

const MaxDescription = 1024

const (
	noServicesDescription = "Access the user's services and accounts through the pandora CLI. No services are connected yet. Use it when the user asks to connect a service, an API or an account: MCP servers, HTTP APIs with or without an OpenAPI spec, or any command line tool."
	descriptionPrefix     = "Access the user's connected services and accounts through the pandora CLI: "
	descriptionSuffix     = ". Use it whenever the user asks about these services, their data or actions in them, or asks to connect another service or account."
	eventsSuffix          = ". Use it whenever the user asks about these services, their data, actions in them or the events they sent, like new comments or messages, or asks to connect another service or account."
)

func Description(doc Doc) string {
	services := normalizedServices(doc.Services)
	if len(services) == 0 {
		return noServicesDescription
	}
	suffix := descriptionSuffix
	if hasWebhooks(services) {
		suffix = eventsSuffix
	}
	full := make([]string, len(services))
	compact := make([]string, len(services))
	for i, service := range services {
		full[i] = serviceWithAccounts(service)
		compact[i] = serviceWithAccountCount(service)
	}
	if description := descriptionPrefix + strings.Join(full, ", ") + suffix; len(description) <= MaxDescription {
		return description
	}
	if description := descriptionPrefix + strings.Join(compact, ", ") + suffix; len(description) <= MaxDescription {
		return description
	}
	best := ""
	for included := 0; included <= len(compact); included++ {
		var list string
		if included == 0 {
			list = fmt.Sprintf("and %d more services", len(compact))
		} else {
			list = strings.Join(compact[:included], ", ")
			if included < len(compact) {
				list += fmt.Sprintf(" and %d more services", len(compact)-included)
			}
		}
		description := descriptionPrefix + list + suffix
		if len(description) <= MaxDescription {
			best = description
		}
	}
	return best
}

func normalizedServices(services []Service) []Service {
	out := make([]Service, len(services))
	for i, service := range services {
		out[i] = service
		out[i].Accounts = append([]Account(nil), service.Accounts...)
		out[i].Tools = append([]Tool(nil), service.Tools...)
		sort.SliceStable(out[i].Accounts, func(a, b int) bool {
			return out[i].Accounts[a].Name < out[i].Accounts[b].Name
		})
		sort.SliceStable(out[i].Tools, func(a, b int) bool {
			return out[i].Tools[a].Name < out[i].Tools[b].Name
		})
	}
	sort.SliceStable(out, func(a, b int) bool {
		return out[a].Name < out[b].Name
	})
	return out
}

func serviceWithAccounts(service Service) string {
	if len(service.Accounts) == 0 {
		return service.Name + " (no accounts)"
	}
	names := make([]string, len(service.Accounts))
	for i, account := range service.Accounts {
		names[i] = account.Name
	}
	return service.Name + " (accounts: " + strings.Join(names, ", ") + ")"
}

func serviceWithAccountCount(service Service) string {
	count := len(service.Accounts)
	noun := "accounts"
	if count == 1 {
		noun = "account"
	}
	return fmt.Sprintf("%s (%d %s)", service.Name, count, noun)
}
