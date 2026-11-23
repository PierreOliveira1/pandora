package registry_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
)

type fixture struct {
	store   *config.Store
	secrets secrets.Store
	reg     *registry.Registry
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	store := config.NewStore(config.Paths{
		ConfigDir: filepath.Join(root, "config"),
		CacheDir:  filepath.Join(root, "cache"),
		StateDir:  filepath.Join(root, "state"),
	})
	sec := secrets.NewMemoryStore()
	return &fixture{store: store, secrets: sec, reg: registry.New(store, sec)}
}

func TestFirstAccountOfAServiceBecomesItsDefault(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service: "gh",
		Account: "personal",
		Kind:    config.KindMCPStdio,
		Command: []string{"npx", "-y", "gh-mcp"},
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}

	accounts, emptyServices, err := f.reg.List()
	if err != nil {
		t.Fatalf("listing accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accounts))
	}
	if len(emptyServices) != 0 {
		t.Fatalf("got empty services %v, want none", emptyServices)
	}
	if !accounts[0].Default {
		t.Fatal("first account is not the default")
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if got := cfg.Services["gh"].DefaultAccount; got != "personal" {
		t.Fatalf("DefaultAccount = %q, want %q", got, "personal")
	}
}

func TestASecondAccountReusesTheServiceMayRepeatItsDefinitionButCannotChangeIt(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service:       "meta",
		Account:       "store-a",
		Kind:          config.KindMCPHTTP,
		URL:           "https://mcp.example.com/ads",
		OAuthClientID: "client-1",
		OAuthScopes:   []string{"ads_read"},
	})
	if err != nil {
		t.Fatalf("adding first account: %v", err)
	}
	if err := f.reg.Add(registry.AddInput{Service: "meta", Account: "store-b"}); err != nil {
		t.Fatalf("adding second account: %v", err)
	}

	accounts, _, err := f.reg.List()
	if err != nil {
		t.Fatalf("listing accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accounts))
	}
	for _, account := range accounts {
		if account.Kind != config.KindMCPHTTP {
			t.Errorf("%s kind = %q, want %q", account.Account, account.Kind, config.KindMCPHTTP)
		}
		if account.Auth != config.AuthOAuth {
			t.Errorf("%s auth = %q, want %q", account.Account, account.Auth, config.AuthOAuth)
		}
	}

	err = f.reg.Add(registry.AddInput{
		Service: "meta",
		Account: "store-c",
		URL:     "https://other.example.com/ads",
	})
	if err == nil || !strings.Contains(err.Error(), `service "meta" already exists with a different definition`) {
		t.Fatalf("redefining service got error %v, want a different definition error", err)
	}

	err = f.reg.Add(registry.AddInput{
		Service: "meta",
		Account: "store-d",
		Kind:    config.KindMCPHTTP,
		URL:     "https://mcp.example.com/ads",
	})
	if err != nil {
		t.Fatalf("repeating the same definition got error %v, want the account added", err)
	}

	err = f.reg.Add(registry.AddInput{Service: "meta", Account: "store-b"})
	want := `account "meta/store-b" already exists`
	if err == nil || err.Error() != want {
		t.Fatalf("adding the same account got error %v, want %q", err, want)
	}
}

func TestAccountSecretsGoToTheSecretStoreAndNeverToConfigTOML(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service:       "gh",
		Account:       "work",
		Kind:          config.KindMCPHTTP,
		URL:           "https://api.example.com/mcp",
		SecretEnv:     map[string]string{"TOKEN": "env-secret-value"},
		SecretHeaders: map[string]string{"authorization": "Bearer header-secret-value"},
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}

	data, err := os.ReadFile(f.store.Paths().ConfigFile())
	if err != nil {
		t.Fatalf("reading config.toml: %v", err)
	}
	for _, secret := range []string{"env-secret-value", "header-secret-value"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("config.toml contains secret %q", secret)
		}
	}

	value, err := f.secrets.Get(secrets.EnvKey("gh", "work", "TOKEN"))
	if err != nil {
		t.Fatalf("reading env secret: %v", err)
	}
	if value != "env-secret-value" {
		t.Fatalf("env secret = %q, want %q", value, "env-secret-value")
	}
	value, err = f.secrets.Get(secrets.HeaderKey("gh", "work", "Authorization"))
	if err != nil {
		t.Fatalf("reading header secret: %v", err)
	}
	if value != "Bearer header-secret-value" {
		t.Fatalf("header secret = %q, want %q", value, "Bearer header-secret-value")
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	account := cfg.Services["gh"].Accounts["work"]
	if !slices.Equal(account.SecretEnv, []string{"TOKEN"}) {
		t.Fatalf("SecretEnv = %v, want [TOKEN]", account.SecretEnv)
	}
	if !slices.Equal(account.SecretHeaders, []string{"Authorization"}) {
		t.Fatalf("SecretHeaders = %v, want [Authorization]", account.SecretHeaders)
	}
	if got := cfg.Services["gh"].Auth; got != config.AuthHeader {
		t.Fatalf("service auth = %q, want %q", got, config.AuthHeader)
	}
}

func TestRenamingAnAccountKeepsItsSecretsReachable(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service:       "gh",
		Account:       "personal",
		Kind:          config.KindMCPHTTP,
		URL:           "https://api.example.com/mcp",
		SecretEnv:     map[string]string{"TOKEN": "abc"},
		SecretHeaders: map[string]string{"Authorization": "Bearer xyz"},
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}
	if err := f.secrets.Set(secrets.OAuthTokenKey("gh", "personal"), "token-123"); err != nil {
		t.Fatalf("setting oauth token: %v", err)
	}

	err = f.reg.RenameAccount(registry.Ref{Service: "gh", Account: "personal"}, "work")
	if err != nil {
		t.Fatalf("renaming account: %v", err)
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if _, ok := cfg.Services["gh"].Accounts["personal"]; ok {
		t.Fatal("old account still exists")
	}
	if _, ok := cfg.Services["gh"].Accounts["work"]; !ok {
		t.Fatal("renamed account is missing")
	}
	if got := cfg.Services["gh"].DefaultAccount; got != "work" {
		t.Fatalf("DefaultAccount = %q, want %q", got, "work")
	}

	checks := map[string]string{
		secrets.EnvKey("gh", "work", "TOKEN"):            "abc",
		secrets.HeaderKey("gh", "work", "Authorization"): "Bearer xyz",
		secrets.OAuthTokenKey("gh", "work"):              "token-123",
	}
	for key, want := range checks {
		got, err := f.secrets.Get(key)
		if err != nil {
			t.Errorf("reading %s: %v", key, err)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if _, err := f.secrets.Get(secrets.EnvKey("gh", "personal", "TOKEN")); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("old env secret still reachable: %v", err)
	}
}

func TestRemovingTheDefaultAccountPromotesAnotherAccount(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service:   "gh",
		Account:   "zulu",
		Kind:      config.KindCommand,
		SecretEnv: map[string]string{"TOKEN": "zulu-secret"},
	})
	if err != nil {
		t.Fatalf("adding first account: %v", err)
	}
	if err := f.reg.Add(registry.AddInput{Service: "gh", Account: "alpha"}); err != nil {
		t.Fatalf("adding second account: %v", err)
	}

	err = f.reg.RemoveAccount(registry.Ref{Service: "gh", Account: "zulu"})
	if err != nil {
		t.Fatalf("removing default account: %v", err)
	}

	accounts, _, err := f.reg.List()
	if err != nil {
		t.Fatalf("listing accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Account != "alpha" {
		t.Fatalf("accounts = %+v, want only alpha", accounts)
	}
	if !accounts[0].Default {
		t.Fatal("alpha did not become the default")
	}
	if _, err := f.secrets.Get(secrets.EnvKey("gh", "zulu", "TOKEN")); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("removed account secret still reachable: %v", err)
	}
}

func TestRemovingAServiceDeletesTheSecretsOfAllItsAccounts(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service:           "gh",
		Account:           "a",
		Kind:              config.KindMCPHTTP,
		URL:               "https://api.example.com/mcp",
		OAuthClientID:     "client-1",
		OAuthClientSecret: "client-secret",
		SecretEnv:         map[string]string{"TOKEN": "a-secret"},
	})
	if err != nil {
		t.Fatalf("adding first account: %v", err)
	}
	err = f.reg.Add(registry.AddInput{
		Service:       "gh",
		Account:       "b",
		SecretEnv:     map[string]string{"TOKEN": "b-secret"},
		SecretHeaders: map[string]string{"Authorization": "Bearer b-secret"},
	})
	if err != nil {
		t.Fatalf("adding second account: %v", err)
	}
	if err := f.secrets.Set(secrets.OAuthTokenKey("gh", "a"), "token-a"); err != nil {
		t.Fatalf("setting oauth token: %v", err)
	}

	err = f.reg.RemoveService("gh")
	if err != nil {
		t.Fatalf("removing service: %v", err)
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if _, ok := cfg.Services["gh"]; ok {
		t.Fatal("service still exists")
	}

	keys := []string{
		secrets.EnvKey("gh", "a", "TOKEN"),
		secrets.OAuthTokenKey("gh", "a"),
		secrets.OAuthClientKey("gh"),
		secrets.EnvKey("gh", "b", "TOKEN"),
		secrets.HeaderKey("gh", "b", "Authorization"),
	}
	for _, key := range keys {
		if _, err := f.secrets.Get(key); !errors.Is(err, secrets.ErrNotFound) {
			t.Errorf("secret %s still reachable: %v", key, err)
		}
	}
}

func TestAServiceRefWithoutAccountResolvesToTheDefaultAccount(t *testing.T) {
	f := newFixture(t)
	if err := f.reg.Add(registry.AddInput{Service: "gh", Account: "personal", Kind: config.KindCommand}); err != nil {
		t.Fatalf("adding first account: %v", err)
	}

	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	resolved, err := registry.Resolve(cfg, registry.Ref{Service: "gh"})
	if err != nil {
		t.Fatalf("resolving default account: %v", err)
	}
	if resolved.String() != "gh/personal" {
		t.Fatalf("resolved = %q, want %q", resolved.String(), "gh/personal")
	}

	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Services["gh"].DefaultAccount = ""
		return nil
	}); err != nil {
		t.Fatalf("clearing default account: %v", err)
	}
	if err := f.reg.Add(registry.AddInput{Service: "gh", Account: "work"}); err != nil {
		t.Fatalf("adding second account: %v", err)
	}
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Services["gh"].DefaultAccount = ""
		return nil
	}); err != nil {
		t.Fatalf("clearing default account: %v", err)
	}

	cfg, err = f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	_, err = registry.Resolve(cfg, registry.Ref{Service: "gh"})
	if err == nil || !strings.Contains(err.Error(), "has no default account") {
		t.Fatalf("resolving without a default got error %v, want a no-default error", err)
	}

	resolved, err = registry.Resolve(cfg, registry.Ref{Service: "gh", Account: "work"})
	if err != nil {
		t.Fatalf("resolving explicit account: %v", err)
	}
	if resolved.Account != "work" {
		t.Fatalf("resolved account = %q, want %q", resolved.Account, "work")
	}
}
