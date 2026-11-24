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
		next := ApplyPolicy(*account, in)
		*account = next
		result = cloneAccount(next)
		return nil
	})
	if err != nil {
		return config.Account{}, err
	}
	return result, nil
}

func ApplyPolicy(account config.Account, in PolicyInput) config.Account {
	next := cloneAccount(account)
	if in.Clear {
		next.ReadOnly = false
		next.Allow = nil
		next.Deny = nil
	}
	if in.ReadOnly != nil {
		next.ReadOnly = *in.ReadOnly
	}
	next.Allow = applyRules(next.Allow, in.Allow, in.RemoveRule)
	next.Deny = applyRules(next.Deny, in.Deny, in.RemoveRule)
	return next
}

func Loosens(before, after config.Account) bool {
	if before.ReadOnly && !after.ReadOnly {
		return true
	}
	if hasRuleOutside(before.Deny, after.Deny) {
		return true
	}
	if len(before.Allow) > 0 {
		if len(after.Allow) == 0 {
			return true
		}
		if hasRuleOutside(after.Allow, before.Allow) {
			return true
		}
	}
	return false
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

func hasRuleOutside(rules, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, rule := range allowed {
		set[rule] = struct{}{}
	}
	for _, rule := range rules {
		if _, ok := set[rule]; !ok {
			return true
		}
	}
	return false
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
