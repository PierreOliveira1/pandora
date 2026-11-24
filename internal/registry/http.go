package registry

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
)

type HTTPServiceInfo struct {
	Service string
	URLs    map[string]string
	Headers map[string]string
	Tools   []config.CommandTool
}

type AccountEnvInfo struct {
	Env       map[string]string
	SecretEnv []string
}

func (r *Registry) SetServiceURL(service, name, raw string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := httptool.ValidateURL(raw); err != nil {
		return err
	}
	raw = strings.TrimSuffix(raw, "/")
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	if _, err := checkHTTPService(cfg, service); err != nil {
		return err
	}
	err = r.store.Update(func(cfg *config.Config) error {
		value, err := checkHTTPService(cfg, service)
		if err != nil {
			return err
		}
		if value.URLs == nil {
			value.URLs = map[string]string{}
		}
		value.URLs[name] = raw
		return nil
	})
	if err != nil {
		return err
	}
	return r.clearToolCache(service)
}

func (r *Registry) RemoveServiceURL(service, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	value, err := checkHTTPService(cfg, service)
	if err != nil {
		return err
	}
	if _, ok := value.URLs[name]; !ok {
		return fmt.Errorf("URL %s does not exist in service %s", name, service)
	}
	err = r.store.Update(func(cfg *config.Config) error {
		value, err := checkHTTPService(cfg, service)
		if err != nil {
			return err
		}
		if _, ok := value.URLs[name]; !ok {
			return fmt.Errorf("URL %s does not exist in service %s", name, service)
		}
		if len(value.URLs) == 1 {
			return fmt.Errorf("an HTTP service needs at least one URL; remove the service with pandora remove %s --yes instead", service)
		}
		for _, tool := range value.Tools {
			if tool.HTTP == nil {
				continue
			}
			if tool.HTTP.URL == name || (tool.HTTP.URL == "" && len(value.URLs) == 1) {
				return fmt.Errorf("tool %s uses URL %s: remove the tool first", tool.Name, name)
			}
		}
		delete(value.URLs, name)
		return nil
	})
	if err != nil {
		return err
	}
	return r.clearToolCache(service)
}

func (r *Registry) SetServiceHeader(service, name, template string) error {
	if err := httptool.ValidateHeader(name, template); err != nil {
		return err
	}
	canonical := http.CanonicalHeaderKey(name)
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	if _, err := checkHTTPService(cfg, service); err != nil {
		return err
	}
	return r.store.Update(func(cfg *config.Config) error {
		value, err := checkHTTPService(cfg, service)
		if err != nil {
			return err
		}
		if value.Headers == nil {
			value.Headers = map[string]string{}
		}
		value.Headers[canonical] = template
		return nil
	})
}

func (r *Registry) RemoveServiceHeader(service, name string) error {
	canonical := http.CanonicalHeaderKey(name)
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	value, err := checkHTTPService(cfg, service)
	if err != nil {
		return err
	}
	if _, ok := value.Headers[canonical]; !ok {
		return fmt.Errorf("header %s does not exist in service %s", canonical, service)
	}
	return r.store.Update(func(cfg *config.Config) error {
		value, err := checkHTTPService(cfg, service)
		if err != nil {
			return err
		}
		if _, ok := value.Headers[canonical]; !ok {
			return fmt.Errorf("header %s does not exist in service %s", canonical, service)
		}
		delete(value.Headers, canonical)
		return nil
	})
}

func (r *Registry) SetAccountEnv(ref Ref, name, value string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if err := validateEnvName(name); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return err
	}
	account := cfg.Services[resolved.Service].Accounts[resolved.Account]
	if containsName(account.SecretEnv, name) {
		return secretEnvError(name, resolved)
	}
	return r.store.Update(func(cfg *config.Config) error {
		resolved, err := Resolve(cfg, ref)
		if err != nil {
			return err
		}
		account, ok := cfg.Services[resolved.Service].Accounts[resolved.Account]
		if !ok {
			return fmt.Errorf("unknown account %q", resolved.String())
		}
		if containsName(account.SecretEnv, name) {
			return secretEnvError(name, resolved)
		}
		if account.Env == nil {
			account.Env = map[string]string{}
		}
		account.Env[name] = value
		return nil
	})
}

func (r *Registry) UnsetAccountEnv(ref Ref, name string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if err := validateEnvName(name); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return err
	}
	account := cfg.Services[resolved.Service].Accounts[resolved.Account]
	if containsName(account.SecretEnv, name) {
		return secretEnvError(name, resolved)
	}
	if _, ok := account.Env[name]; !ok {
		return fmt.Errorf("%s is not set for %s", name, resolved)
	}
	return r.store.Update(func(cfg *config.Config) error {
		resolved, err := Resolve(cfg, ref)
		if err != nil {
			return err
		}
		account, ok := cfg.Services[resolved.Service].Accounts[resolved.Account]
		if !ok {
			return fmt.Errorf("unknown account %q", resolved.String())
		}
		if containsName(account.SecretEnv, name) {
			return secretEnvError(name, resolved)
		}
		if _, ok := account.Env[name]; !ok {
			return fmt.Errorf("%s is not set for %s", name, resolved)
		}
		delete(account.Env, name)
		return nil
	})
}

func (r *Registry) GetHTTPService(service string) (HTTPServiceInfo, error) {
	if err := ValidateName(service); err != nil {
		return HTTPServiceInfo{}, err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return HTTPServiceInfo{}, err
	}
	value, err := checkHTTPService(cfg, service)
	if err != nil {
		return HTTPServiceInfo{}, err
	}
	info := HTTPServiceInfo{
		Service: service,
		URLs:    cloneStringMap(value.URLs),
		Headers: cloneStringMap(value.Headers),
		Tools:   make([]config.CommandTool, 0, len(value.Tools)),
	}
	for _, tool := range value.Tools {
		info.Tools = append(info.Tools, cloneCommandTool(tool))
	}
	return info, nil
}

func (r *Registry) GetAccountEnv(ref Ref) (AccountEnvInfo, error) {
	if err := validateRef(ref); err != nil {
		return AccountEnvInfo{}, err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return AccountEnvInfo{}, err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return AccountEnvInfo{}, err
	}
	account, ok := cfg.Services[resolved.Service].Accounts[resolved.Account]
	if !ok {
		return AccountEnvInfo{}, fmt.Errorf("unknown account %q", resolved.String())
	}
	return AccountEnvInfo{
		Env:       cloneStringMap(account.Env),
		SecretEnv: append([]string(nil), account.SecretEnv...),
	}, nil
}

func checkHTTPService(cfg *config.Config, service string) (*config.Service, error) {
	value, ok := cfg.Services[service]
	if !ok {
		return nil, fmt.Errorf("unknown service %q", service)
	}
	if value.Kind != config.KindHTTP {
		return nil, httpServiceError(service)
	}
	return value, nil
}

func httpServiceError(service string) error {
	return fmt.Errorf("service %s is not an HTTP service: create one with pandora add %s --as <account> --http name=URL", service, service)
}

func validateEnvName(name string) error {
	if name == "" {
		return errors.New("environment name is empty")
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return fmt.Errorf("invalid environment name %q: use letters, digits and underscores, starting with a letter or underscore", name)
	}
	return nil
}

func containsName(names []string, name string) bool {
	for _, value := range names {
		if value == name {
			return true
		}
	}
	return false
}

func secretEnvError(name string, ref Ref) error {
	return fmt.Errorf("%s is a secret of %s: change it with pandora remove and add", name, ref)
}

func normalizedURLs(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, raw := range values {
		out[name] = strings.TrimSuffix(raw, "/")
	}
	return out
}

func sameURLs(existing, incoming map[string]string) bool {
	if len(existing) != len(incoming) {
		return false
	}
	for name, raw := range incoming {
		value, ok := existing[name]
		if !ok || strings.TrimSuffix(value, "/") != strings.TrimSuffix(raw, "/") {
			return false
		}
	}
	return true
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

func cloneCommandTool(tool config.CommandTool) config.CommandTool {
	copied := tool
	copied.Command = append([]string(nil), tool.Command...)
	if len(tool.Params) > 0 {
		copied.Params = make(map[string]config.CommandParam, len(tool.Params))
		for name, parameter := range tool.Params {
			copied.Params[name] = parameter
		}
	}
	if tool.HTTP != nil {
		spec := *tool.HTTP
		spec.Query = cloneStringMap(tool.HTTP.Query)
		spec.Headers = cloneStringMap(tool.HTTP.Headers)
		spec.Form = cloneStringMap(tool.HTTP.Form)
		spec.JSON = cloneStringMap(tool.HTTP.JSON)
		copied.HTTP = &spec
	}
	return copied
}
