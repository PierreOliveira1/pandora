package registry_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func TestPolicyRulesAreValidatedAndKeptWithoutDuplicates(t *testing.T) {
	f := newFixture(t)
	if err := f.reg.Add(registry.AddInput{
		Service: "todoist",
		Account: "work",
		Kind:    config.KindMCPHTTP,
		URL:     "https://todoist.example.com/mcp",
	}); err != nil {
		t.Fatalf("adding account: %v", err)
	}
	ref := registry.Ref{Service: "todoist", Account: "work"}

	_, err := f.reg.SetPolicy(ref, registry.PolicyInput{Allow: []string{"["}})
	if err == nil || !strings.Contains(err.Error(), `invalid pattern "["`) {
		t.Fatalf("SetPolicy() error = %v, want an invalid pattern error", err)
	}

	readOnly := true
	account, err := f.reg.SetPolicy(ref, registry.PolicyInput{
		ReadOnly: &readOnly,
		Allow:    []string{"get-*", "find-*", "get-*"},
		Deny:     []string{"delete-*", "delete-*"},
	})
	if err != nil {
		t.Fatalf("SetPolicy() error = %v", err)
	}
	if !account.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
	if want := []string{"find-*", "get-*"}; !reflect.DeepEqual(account.Allow, want) {
		t.Errorf("Allow = %v, want %v", account.Allow, want)
	}
	if want := []string{"delete-*"}; !reflect.DeepEqual(account.Deny, want) {
		t.Errorf("Deny = %v, want %v", account.Deny, want)
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	stored := cfg.Services["todoist"].Accounts["work"]
	if !stored.ReadOnly {
		t.Error("stored ReadOnly = false, want true")
	}
	if want := []string{"find-*", "get-*"}; !reflect.DeepEqual(stored.Allow, want) {
		t.Errorf("stored Allow = %v, want %v", stored.Allow, want)
	}
	if want := []string{"delete-*"}; !reflect.DeepEqual(stored.Deny, want) {
		t.Errorf("stored Deny = %v, want %v", stored.Deny, want)
	}
}
