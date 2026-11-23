package registry

import (
	"fmt"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
)

type CommandToolInput struct {
	Service     string
	Name        string
	Description string
	Command     []string
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
	if len(in.Command) == 0 {
		return fmt.Errorf("tool %q needs a command", in.Name)
	}
	if err := validateCommandParams(in.Name, in.Params); err != nil {
		return err
	}
	if err := validateCommandPlaceholders(in.Name, in.Command, in.Params); err != nil {
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
	if service.Kind != config.KindCommand {
		return fmt.Errorf("service %q is not a command service", in.Service)
	}
	if hasCommandTool(service, in.Name) {
		return fmt.Errorf("tool %q already exists in service %q", in.Name, in.Service)
	}

	return r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[in.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", in.Service)
		}
		if service.Kind != config.KindCommand {
			return fmt.Errorf("service %q is not a command service", in.Service)
		}
		if hasCommandTool(service, in.Name) {
			return fmt.Errorf("tool %q already exists in service %q", in.Name, in.Service)
		}
		service.Tools = append(service.Tools, config.CommandTool{
			Name:        in.Name,
			Description: in.Description,
			Command:     in.Command,
			Params:      in.Params,
			ReadOnly:    in.ReadOnly,
			TimeoutSec:  in.TimeoutSec,
		})
		return nil
	})
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
	if value.Kind != config.KindCommand {
		return fmt.Errorf("service %q is not a command service", service)
	}
	if !hasCommandTool(value, name) {
		return fmt.Errorf("tool %q not found in service %q", name, service)
	}

	return r.store.Update(func(cfg *config.Config) error {
		value, ok := cfg.Services[service]
		if !ok {
			return fmt.Errorf("unknown service %q", service)
		}
		if value.Kind != config.KindCommand {
			return fmt.Errorf("service %q is not a command service", service)
		}
		for i := range value.Tools {
			if value.Tools[i].Name != name {
				continue
			}
			value.Tools = append(value.Tools[:i], value.Tools[i+1:]...)
			return nil
		}
		return fmt.Errorf("tool %q not found in service %q", name, service)
	})
}

func hasCommandTool(service *config.Service, name string) bool {
	for _, tool := range service.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
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
