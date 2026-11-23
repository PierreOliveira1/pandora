package registry

import (
	"fmt"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
)

const maxNameLength = 32

func ValidateName(name string) error {
	valid := len(name) > 0 && len(name) <= maxNameLength
	for i := 0; valid && i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' && i > 0:
		default:
			valid = false
		}
	}
	if !valid {
		return fmt.Errorf("invalid name %q: use lowercase letters, digits and dashes (max 32)", name)
	}
	return nil
}

type Ref struct {
	Service string
	Account string
}

func ParseRef(s string) (Ref, error) {
	parts := strings.Split(s, "/")
	switch len(parts) {
	case 1:
		if err := ValidateName(parts[0]); err != nil {
			return Ref{}, err
		}
		return Ref{Service: parts[0]}, nil
	case 2:
		if err := ValidateName(parts[0]); err != nil {
			return Ref{}, err
		}
		if err := ValidateName(parts[1]); err != nil {
			return Ref{}, err
		}
		return Ref{Service: parts[0], Account: parts[1]}, nil
	default:
		return Ref{}, fmt.Errorf("invalid ref %q: use service or service/account", s)
	}
}

func (r Ref) String() string {
	if r.Account == "" {
		return r.Service
	}
	return r.Service + "/" + r.Account
}

func Resolve(cfg *config.Config, r Ref) (Ref, error) {
	service, ok := cfg.Services[r.Service]
	if !ok {
		return Ref{}, fmt.Errorf("unknown service %q", r.Service)
	}

	if r.Account == "" {
		switch {
		case service.DefaultAccount != "":
			r.Account = service.DefaultAccount
		case len(service.Accounts) == 1:
			for account := range service.Accounts {
				r.Account = account
			}
		default:
			return Ref{}, fmt.Errorf("service %q has no default account: use %s/<account>", r.Service, r.Service)
		}
	}

	if _, ok := service.Accounts[r.Account]; !ok {
		return Ref{}, fmt.Errorf("unknown account %q", r.String())
	}
	return r, nil
}
