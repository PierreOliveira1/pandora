package registry

import (
	"fmt"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func (r *Registry) SetExpose(service string, mode config.ExposeMode) error {
	if mode != config.ExposeCompact && mode != config.ExposeDirect {
		return fmt.Errorf("invalid expose mode %q: use compact or direct", mode)
	}
	if err := ValidateName(service); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Services[service]; !ok {
		return fmt.Errorf("unknown service %q", service)
	}
	return r.store.Update(func(cfg *config.Config) error {
		serviceConfig, ok := cfg.Services[service]
		if !ok {
			return fmt.Errorf("unknown service %q", service)
		}
		serviceConfig.Expose = mode
		return nil
	})
}
