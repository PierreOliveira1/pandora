package registry_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
)

func addTestAccount(t *testing.T, f *fixture, service, account string) {
	t.Helper()
	if err := f.reg.Add(registry.AddInput{
		Service: service,
		Account: account,
		Kind:    config.KindCommand,
	}); err != nil {
		t.Fatalf("adding %s/%s: %v", service, account, err)
	}
}

func loadConfig(t *testing.T, f *fixture) *config.Config {
	t.Helper()
	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	return cfg
}

func secretAt(t *testing.T, f *fixture, key string) string {
	t.Helper()
	value, err := f.secrets.Get(key)
	if err != nil {
		t.Fatalf("reading secret %s: %v", key, err)
	}
	return value
}

func noSecretAt(t *testing.T, f *fixture, key string) {
	t.Helper()
	if _, err := f.secrets.Get(key); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("secret %s still exists: %v", key, err)
	}
}

func TestAddingAnHMACWebhookWithHubStoresTheSecretAndReturnsTheVerifyToken(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")

	added, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:       registry.Ref{Service: "ig"},
		Style:     "hmac",
		Challenge: "hub",
		Secret:    "app-secret",
	})
	if err != nil {
		t.Fatalf("AddWebhook() error = %v", err)
	}
	if added.Ref != (registry.Ref{Service: "ig", Account: "tron"}) {
		t.Errorf("Ref = %s, want ig/tron", added.Ref)
	}
	if added.Webhook.Style != "hmac" || added.Webhook.Challenge != "hub" {
		t.Errorf("Webhook style/challenge = %s/%s, want hmac/hub", added.Webhook.Style, added.Webhook.Challenge)
	}
	if len(added.Webhook.ID) != 32 {
		t.Errorf("webhook ID = %q, want 32 hex characters", added.Webhook.ID)
	}
	if len(added.VerifyToken) != 48 {
		t.Errorf("verify token = %q, want 48 hex characters", added.VerifyToken)
	}
	if added.GeneratedToken != "" {
		t.Errorf("GeneratedToken = %q, want empty", added.GeneratedToken)
	}
	if got := secretAt(t, f, secrets.WebhookSigningKey("ig", "tron")); got != "app-secret" {
		t.Errorf("signing secret = %q, want app-secret", got)
	}
	if got := secretAt(t, f, secrets.WebhookVerifyTokenKey("ig", "tron")); got != added.VerifyToken {
		t.Errorf("verify token secret = %q, want %q", got, added.VerifyToken)
	}

	cfg := loadConfig(t, f)
	hook := cfg.Services["ig"].Accounts["tron"].Webhook
	if hook == nil {
		t.Fatal("saved account has no webhook")
	}
	if !reflect.DeepEqual(*hook, added.Webhook) {
		t.Errorf("saved webhook = %+v, want %+v", *hook, added.Webhook)
	}
}

func TestAnAccountCannotHaveTwoWebhooks(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")
	if _, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:    registry.Ref{Service: "ig", Account: "tron"},
		Style:  "hmac",
		Secret: "first",
	}); err != nil {
		t.Fatalf("first AddWebhook() error = %v", err)
	}

	_, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:    registry.Ref{Service: "ig", Account: "tron"},
		Style:  "hmac",
		Secret: "second",
	})
	want := "ig/tron already has a webhook: remove it with pandora webhook remove ig/tron first"
	if err == nil || err.Error() != want {
		t.Fatalf("second AddWebhook() error = %v, want %q", err, want)
	}
}

func TestNoneStyleOnlyWorksWithInsecure(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")

	_, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:   registry.Ref{Service: "ig", Account: "tron"},
		Style: "none",
	})
	want := "--style none accepts deliveries from anyone: add --insecure if that is what you want"
	if err == nil || err.Error() != want {
		t.Fatalf("AddWebhook() error = %v, want %q", err, want)
	}

	added, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:      registry.Ref{Service: "ig", Account: "tron"},
		Style:    "none",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("AddWebhook() with insecure error = %v", err)
	}
	if added.Webhook.Style != "none" {
		t.Errorf("webhook style = %q, want none", added.Webhook.Style)
	}
}

func TestHMACWithoutASecretIsRefused(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")

	_, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:   registry.Ref{Service: "ig", Account: "tron"},
		Style: "hmac",
	})
	want := "the signing secret is required for --style hmac"
	if err == nil || err.Error() != want {
		t.Fatalf("AddWebhook() error = %v, want %q", err, want)
	}
	cfg := loadConfig(t, f)
	if cfg.Services["ig"].Accounts["tron"].Webhook != nil {
		t.Error("account has a webhook after a failed add")
	}
}

func TestATokenWebhookWithoutASecretGeneratesOne(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")

	added, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:         registry.Ref{Service: "ig", Account: "tron"},
		Style:       "token",
		TokenHeader: "X-Token",
	})
	if err != nil {
		t.Fatalf("AddWebhook() error = %v", err)
	}
	if len(added.GeneratedToken) != 64 {
		t.Errorf("GeneratedToken = %q, want 64 hex characters", added.GeneratedToken)
	}
	if added.VerifyToken != "" {
		t.Errorf("VerifyToken = %q, want empty without hub", added.VerifyToken)
	}
	if got := secretAt(t, f, secrets.WebhookSigningKey("ig", "tron")); got != added.GeneratedToken {
		t.Errorf("signing secret = %q, want the generated token", got)
	}
	if added.Webhook.TokenHeader != "X-Token" {
		t.Errorf("TokenHeader = %q, want X-Token", added.Webhook.TokenHeader)
	}
}

func TestRemovingAWebhookDeletesItsSecrets(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")
	added, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:       registry.Ref{Service: "ig", Account: "tron"},
		Style:     "hmac",
		Challenge: "hub",
		Secret:    "app-secret",
	})
	if err != nil {
		t.Fatalf("AddWebhook() error = %v", err)
	}

	ref, err := f.reg.RemoveWebhook(registry.Ref{Service: "ig"})
	if err != nil {
		t.Fatalf("RemoveWebhook() error = %v", err)
	}
	if ref != (registry.Ref{Service: "ig", Account: "tron"}) {
		t.Errorf("RemoveWebhook() ref = %s, want ig/tron", ref)
	}
	cfg := loadConfig(t, f)
	if cfg.Services["ig"].Accounts["tron"].Webhook != nil {
		t.Error("account still has a webhook")
	}
	if added.VerifyToken == "" {
		t.Error("test setup produced no verify token")
	}
	noSecretAt(t, f, secrets.WebhookSigningKey("ig", "tron"))
	noSecretAt(t, f, secrets.WebhookVerifyTokenKey("ig", "tron"))

	if _, err := f.reg.RemoveWebhook(registry.Ref{Service: "ig", Account: "tron"}); err == nil || !strings.Contains(err.Error(), "has no webhook") {
		t.Errorf("second RemoveWebhook() error = %v, want a has no webhook error", err)
	}
}

func TestRemovingAnAccountDeletesItsWebhookSecrets(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")
	if _, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:       registry.Ref{Service: "ig", Account: "tron"},
		Style:     "hmac",
		Challenge: "hub",
		Secret:    "app-secret",
	}); err != nil {
		t.Fatalf("AddWebhook() error = %v", err)
	}

	if err := f.reg.RemoveAccount(registry.Ref{Service: "ig", Account: "tron"}); err != nil {
		t.Fatalf("RemoveAccount() error = %v", err)
	}
	cfg := loadConfig(t, f)
	if _, ok := cfg.Services["ig"].Accounts["tron"]; ok {
		t.Error("account still exists")
	}
	noSecretAt(t, f, secrets.WebhookSigningKey("ig", "tron"))
	noSecretAt(t, f, secrets.WebhookVerifyTokenKey("ig", "tron"))
}

func TestRenamingAnAccountMovesTheWebhookSecretsAndReaderFilters(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")
	added, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:       registry.Ref{Service: "ig", Account: "tron"},
		Style:     "hmac",
		Challenge: "hub",
		Secret:    "app-secret",
	})
	if err != nil {
		t.Fatalf("AddWebhook() error = %v", err)
	}
	if _, err := f.reg.AddReader(config.EventReader{
		Name:    "worker",
		Service: "ig",
		Account: "tron",
		Command: []string{"echo"},
	}); err != nil {
		t.Fatalf("AddReader() error = %v", err)
	}

	if err := f.reg.RenameAccount(registry.Ref{Service: "ig", Account: "tron"}, "loja"); err != nil {
		t.Fatalf("RenameAccount() error = %v", err)
	}

	if got := secretAt(t, f, secrets.WebhookSigningKey("ig", "loja")); got != "app-secret" {
		t.Errorf("new signing secret = %q, want app-secret", got)
	}
	if got := secretAt(t, f, secrets.WebhookVerifyTokenKey("ig", "loja")); got != added.VerifyToken {
		t.Errorf("new verify token = %q, want %q", got, added.VerifyToken)
	}
	noSecretAt(t, f, secrets.WebhookSigningKey("ig", "tron"))
	noSecretAt(t, f, secrets.WebhookVerifyTokenKey("ig", "tron"))

	cfg := loadConfig(t, f)
	account := cfg.Services["ig"].Accounts["loja"]
	if account == nil || account.Webhook == nil {
		t.Fatal("renamed account has no webhook")
	}
	if len(cfg.Readers) != 1 || cfg.Readers[0].Service != "ig" || cfg.Readers[0].Account != "loja" {
		t.Errorf("readers after rename = %+v, want the filter on ig/loja", cfg.Readers)
	}
}

func TestRenamingAServiceMovesTheWebhookSecretsAndReaderFilters(t *testing.T) {
	f := newFixture(t)
	addTestAccount(t, f, "ig", "tron")
	added, err := f.reg.AddWebhook(registry.WebhookInput{
		Ref:       registry.Ref{Service: "ig", Account: "tron"},
		Style:     "hmac",
		Challenge: "hub",
		Secret:    "app-secret",
	})
	if err != nil {
		t.Fatalf("AddWebhook() error = %v", err)
	}
	if _, err := f.reg.AddReader(config.EventReader{
		Name:    "worker",
		Service: "ig",
		Account: "tron",
		Command: []string{"echo"},
	}); err != nil {
		t.Fatalf("AddReader() error = %v", err)
	}

	if err := f.reg.RenameService("ig", "meta"); err != nil {
		t.Fatalf("RenameService() error = %v", err)
	}

	if got := secretAt(t, f, secrets.WebhookSigningKey("meta", "tron")); got != "app-secret" {
		t.Errorf("new signing secret = %q, want app-secret", got)
	}
	if got := secretAt(t, f, secrets.WebhookVerifyTokenKey("meta", "tron")); got != added.VerifyToken {
		t.Errorf("new verify token = %q, want %q", got, added.VerifyToken)
	}
	noSecretAt(t, f, secrets.WebhookSigningKey("ig", "tron"))
	noSecretAt(t, f, secrets.WebhookVerifyTokenKey("ig", "tron"))

	cfg := loadConfig(t, f)
	account := cfg.Services["meta"].Accounts["tron"]
	if account == nil || account.Webhook == nil {
		t.Fatal("renamed service has no webhook")
	}
	if len(cfg.Readers) != 1 || cfg.Readers[0].Service != "meta" || cfg.Readers[0].Account != "tron" {
		t.Errorf("readers after rename = %+v, want the filter on meta/tron", cfg.Readers)
	}
}

func TestAReaderNameCannotBeReused(t *testing.T) {
	f := newFixture(t)
	if _, err := f.reg.AddReader(config.EventReader{Name: "worker", Command: []string{"echo"}}); err != nil {
		t.Fatalf("first AddReader() error = %v", err)
	}

	_, err := f.reg.AddReader(config.EventReader{Name: "worker", Command: []string{"true"}})
	want := "reader worker already exists: remove it with pandora events reader remove worker first"
	if err == nil || err.Error() != want {
		t.Fatalf("second AddReader() error = %v, want %q", err, want)
	}
}

func TestAReaderNeedsACommand(t *testing.T) {
	f := newFixture(t)
	_, err := f.reg.AddReader(config.EventReader{Name: "worker"})
	want := "the reader needs a command after --"
	if err == nil || err.Error() != want {
		t.Fatalf("AddReader() error = %v, want %q", err, want)
	}
}

func TestAReaderNeedsAnExistingService(t *testing.T) {
	f := newFixture(t)
	_, err := f.reg.AddReader(config.EventReader{
		Name:    "worker",
		Service: "missing",
		Command: []string{"echo"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("AddReader() error = %v, want an unknown service error", err)
	}
}

func TestAReadersDefaultTimeoutIs60(t *testing.T) {
	f := newFixture(t)
	saved, err := f.reg.AddReader(config.EventReader{Name: "worker", Command: []string{"echo"}})
	if err != nil {
		t.Fatalf("AddReader() error = %v", err)
	}
	if saved.TimeoutSec != 60 {
		t.Errorf("returned timeout = %d, want 60", saved.TimeoutSec)
	}
	cfg := loadConfig(t, f)
	if len(cfg.Readers) != 1 || cfg.Readers[0].TimeoutSec != 60 {
		t.Errorf("saved readers = %+v, want timeout 60", cfg.Readers)
	}
}
