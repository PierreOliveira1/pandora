package registry

import (
	"fmt"
	"path"
	"sort"

	"github.com/PierreOliveira1/pandora/internal/config"
)

type PolicyInput struct {
	ReadOnly   *bool
	Allow      []string
	Deny       []string
	RemoveRule []string
	Clear      bool
}

func (r *Registry) SetPolicy(ref Ref, in PolicyInput) (config.Account, error) {
	if err := validateRef(ref); err != nil {
		return config.Account{}, err
	}
	if err := validatePolicyInput(in); err != nil {
		return config.Account{}, err
	}
	var result config.Account
	err := r.store.Update(func(cfg *config.Config) error {
		resolved, err := Resolve(cfg, ref)
		if err != nil {
			return err
		}
		service, ok := cfg.Services[resolved.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", resolved.Service)
		}
		account, ok := service.Accounts[resolved.Account]
		if !ok {
			return fmt.Errorf("unknown account %q", resolved.String())
		}
		if in.Clear {
			account.ReadOnly = false
			account.Allow = nil
			account.Deny = nil
		}
		if in.ReadOnly != nil {
			account.ReadOnly = *in.ReadOnly
		}
		account.Allow = applyRules(account.Allow, in.Allow, in.RemoveRule)
		account.Deny = applyRules(account.Deny, in.Deny, in.RemoveRule)
		result = cloneAccount(*account)
		return nil
	})
	if err != nil {
		return config.Account{}, err
	}
	return result, nil
}

func (r *Registry) GetPolicy(ref Ref) (config.Account, error) {
	if err := validateRef(ref); err != nil {
		return config.Account{}, err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return config.Account{}, err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return config.Account{}, err
	}
	account, ok := cfg.Services[resolved.Service].Accounts[resolved.Account]
	if !ok {
		return config.Account{}, fmt.Errorf("unknown account %q", resolved.String())
	}
	return cloneAccount(*account), nil
}

func validatePolicyInput(in PolicyInput) error {
	for _, group := range [][]string{in.Allow, in.Deny, in.RemoveRule} {
		for _, pattern := range group {
			if _, err := path.Match(pattern, ""); err != nil {
				return fmt.Errorf("invalid pattern %q", pattern)
			}
		}
	}
	return nil
}

func applyRules(existing, add, remove []string) []string {
	set := make(map[string]bool, len(existing)+len(add))
	for _, rule := range existing {
		set[rule] = true
	}
	for _, rule := range add {
		set[rule] = true
	}
	for _, rule := range remove {
		delete(set, rule)
	}
	if len(set) == 0 {
		return nil
	}
	rules := make([]string, 0, len(set))
	for rule := range set {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	return rules
}

func cloneAccount(account config.Account) config.Account {
	cloned := account
	cloned.Allow = append([]string(nil), account.Allow...)
	cloned.Deny = append([]string(nil), account.Deny...)
	return cloned
}
