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

func TestApplyPolicyBuildsTheNextPolicyFromACopy(t *testing.T) {
	readOnly := true
	tests := []struct {
		name   string
		before config.Account
		input  registry.PolicyInput
		want   config.Account
	}{
		{
			name:   "clear resets the policy and keeps the account data",
			before: config.Account{Description: "Demo", ReadOnly: true, Allow: []string{"get-*"}, Deny: []string{"send-*"}},
			input:  registry.PolicyInput{Clear: true},
			want:   config.Account{Description: "Demo"},
		},
		{
			name:   "readonly sets the flag",
			before: config.Account{Allow: []string{"get-*"}},
			input:  registry.PolicyInput{ReadOnly: &readOnly},
			want:   config.Account{ReadOnly: true, Allow: []string{"get-*"}},
		},
		{
			name:   "rules are added, removed and sorted",
			before: config.Account{Allow: []string{"get-*", "find-*"}, Deny: []string{"send-*"}},
			input:  registry.PolicyInput{Allow: []string{"list-*"}, Deny: []string{"delete-*"}, RemoveRule: []string{"find-*"}},
			want:   config.Account{Allow: []string{"get-*", "list-*"}, Deny: []string{"delete-*", "send-*"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.before
			originalAllow := append([]string(nil), before.Allow...)
			originalDeny := append([]string(nil), before.Deny...)
			got := registry.ApplyPolicy(before, test.input)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("ApplyPolicy() = %+v, want %+v", got, test.want)
			}
			if !reflect.DeepEqual(before.Allow, originalAllow) || !reflect.DeepEqual(before.Deny, originalDeny) {
				t.Errorf("ApplyPolicy() changed the input account: %+v", before)
			}
		})
	}
}

func TestLoosensDetectsOnlyChangesThatMayAllowATool(t *testing.T) {
	tests := []struct {
		name   string
		before config.Account
		after  config.Account
		want   bool
	}{
		{name: "removing a deny loosens", before: config.Account{Deny: []string{"send-*"}}, after: config.Account{}, want: true},
		{name: "making a read-only account writable loosens", before: config.Account{ReadOnly: true}, after: config.Account{}, want: true},
		{name: "clearing the allow loosens", before: config.Account{Allow: []string{"get-*"}}, after: config.Account{}, want: true},
		{name: "adding an allow to an existing list loosens", before: config.Account{Allow: []string{"get-*"}}, after: config.Account{Allow: []string{"get-*", "list-*"}}, want: true},
		{name: "replacing the allow loosens", before: config.Account{Allow: []string{"get-*"}}, after: config.Account{Allow: []string{"list-*"}}, want: true},
		{name: "adding a deny tightens", before: config.Account{}, after: config.Account{Deny: []string{"send-*"}}, want: false},
		{name: "marking read-only tightens", before: config.Account{}, after: config.Account{ReadOnly: true}, want: false},
		{name: "creating the first allow tightens", before: config.Account{}, after: config.Account{Allow: []string{"get-*"}}, want: false},
		{name: "removing an allow from a nonempty list tightens", before: config.Account{Allow: []string{"get-*", "list-*"}}, after: config.Account{Allow: []string{"get-*"}}, want: false},
		{name: "keeping the same policy does not loosen", before: config.Account{ReadOnly: true, Allow: []string{"get-*"}, Deny: []string{"send-*"}}, after: config.Account{ReadOnly: true, Allow: []string{"get-*"}, Deny: []string{"send-*"}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := registry.Loosens(test.before, test.after); got != test.want {
				t.Errorf("Loosens() = %v, want %v", got, test.want)
			}
		})
	}
}
