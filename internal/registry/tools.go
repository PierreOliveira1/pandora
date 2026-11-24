package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
)

type CommandToolInput struct {
	Service     string
	Name        string
	Description string
	Command     []string
	HTTP        *config.HTTPRequest
	Params      map[string]config.CommandParam
	ReadOnly    bool
	TimeoutSec  int
}

func (r *Registry) AddCommandTool(in CommandToolInput) error {
	if err := ValidateName(in.Service); err != nil {
		return err
	}
	if err := ValidateName(in.Name); err != nil {
		return err
	}
	if len(in.Command) > 0 && in.HTTP != nil {
		return fmt.Errorf("tool %q: use a command or an HTTP request, not both", in.Name)
	}
	if len(in.Command) == 0 && in.HTTP == nil {
		return fmt.Errorf("tool %q needs a command or an HTTP request", in.Name)
	}
	if err := validateCommandParams(in.Name, in.Params); err != nil {
		return err
	}
	var httpSpec *config.HTTPRequest
	if in.HTTP != nil {
		copied := *in.HTTP
		copied.Method = strings.ToUpper(copied.Method)
		httpSpec = &copied
	} else if err := validateCommandPlaceholders(in.Name, in.Command, in.Params); err != nil {
		return err
	}

	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	service, ok := cfg.Services[in.Service]
	if !ok {
		return fmt.Errorf("unknown service %q", in.Service)
	}
	if httpSpec != nil && service.Kind != config.KindHTTP {
		return fmt.Errorf("service %s has no URL: add one with pandora http url %s <name> <url>", in.Service, in.Service)
	}
	if httpSpec == nil && service.Kind != config.KindCommand && service.Kind != config.KindHTTP {
		return fmt.Errorf("service %q is not a command or HTTP service", in.Service)
	}
	if hasCommandTool(service, in.Name) {
		return fmt.Errorf("tool %q already exists in service %q", in.Name, in.Service)
	}

	readOnly := in.ReadOnly
	if httpSpec != nil && (httpSpec.Method == http.MethodGet || httpSpec.Method == http.MethodHead) {
		readOnly = true
	}
	tool := config.CommandTool{
		Name:        in.Name,
		Description: in.Description,
		Command:     in.Command,
		HTTP:        httpSpec,
		Params:      in.Params,
		ReadOnly:    readOnly,
		TimeoutSec:  in.TimeoutSec,
	}
	if httpSpec != nil {
		pinSingleURL(httpSpec, service)
		if err := httptool.Validate(tool, service, serviceSecretEnv(service)); err != nil {
			return err
		}
	}

	err = r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[in.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", in.Service)
		}
		if httpSpec != nil && service.Kind != config.KindHTTP {
			return fmt.Errorf("service %s has no URL: add one with pandora http url %s <name> <url>", in.Service, in.Service)
		}
		if httpSpec == nil && service.Kind != config.KindCommand && service.Kind != config.KindHTTP {
			return fmt.Errorf("service %q is not a command or HTTP service", in.Service)
		}
		if hasCommandTool(service, in.Name) {
			return fmt.Errorf("tool %q already exists in service %q", in.Name, in.Service)
		}
		if httpSpec != nil {
			if err := httptool.Validate(tool, service, serviceSecretEnv(service)); err != nil {
				return err
			}
		}
		service.Tools = append(service.Tools, tool)
		return nil
	})
	if err != nil {
		return err
	}
	return r.clearToolCache(in.Service)
}

func (r *Registry) RemoveCommandTool(service, name string) error {
	if err := ValidateName(service); err != nil {
		return err
	}
	if err := ValidateName(name); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	value, ok := cfg.Services[service]
	if !ok {
		return fmt.Errorf("unknown service %q", service)
	}
	if value.Kind != config.KindCommand && value.Kind != config.KindHTTP {
		return fmt.Errorf("service %q is not a command or HTTP service", service)
	}
	if !hasCommandTool(value, name) {
		return fmt.Errorf("tool %q not found in service %q", name, service)
	}

	if err := r.store.Update(func(cfg *config.Config) error {
		value, ok := cfg.Services[service]
		if !ok {
			return fmt.Errorf("unknown service %q", service)
		}
		if value.Kind != config.KindCommand && value.Kind != config.KindHTTP {
			return fmt.Errorf("service %q is not a command or HTTP service", service)
		}
		for i := range value.Tools {
			if value.Tools[i].Name != name {
				continue
			}
			value.Tools = append(value.Tools[:i], value.Tools[i+1:]...)
			return nil
		}
		return fmt.Errorf("tool %q not found in service %q", name, service)
	}); err != nil {
		return err
	}
	return r.clearToolCache(service)
}

func (r *Registry) clearToolCache(service string) error {
	path := r.store.Paths().ToolCacheFile(service)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clearing cached tools of %s: %w; remove %s manually", service, err, path)
	}
	return nil
}

func hasCommandTool(service *config.Service, name string) bool {
	for _, tool := range service.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func pinSingleURL(spec *config.HTTPRequest, service *config.Service) {
	if spec.URL != "" || len(service.URLs) != 1 {
		return
	}
	for name := range service.URLs {
		spec.URL = name
	}
}

func serviceSecretEnv(service *config.Service) map[string]bool {
	names := map[string]bool{}
	for _, account := range service.Accounts {
		if account == nil {
			continue
		}
		for _, name := range account.SecretEnv {
			names[name] = true
		}
	}
	return names
}

func validateCommandParams(toolName string, params map[string]config.CommandParam) error {
	for name, parameter := range params {
		switch parameter.Type {
		case "string", "number", "integer", "boolean", "array":
		default:
			return fmt.Errorf("tool %s: parameter %s has invalid type %q; use string, number, integer, boolean or array", toolName, name, parameter.Type)
		}
	}
	return nil
}

func validateCommandPlaceholders(toolName string, command []string, params map[string]config.CommandParam) error {
	for _, argument := range command {
		remaining := argument
		for {
			open := strings.Index(remaining, "{{")
			if open < 0 {
				break
			}
			after := remaining[open+2:]
			close := strings.Index(after, "}}")
			if close < 0 {
				return fmt.Errorf("tool %s: unknown parameter %s in command", toolName, strings.TrimSpace(after))
			}
			parameter := after[:close]
			if _, ok := params[parameter]; !ok {
				return fmt.Errorf("tool %s: unknown parameter %s in command", toolName, parameter)
			}
			remaining = after[close+2:]
		}
	}
	return nil
}
