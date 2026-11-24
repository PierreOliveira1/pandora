package core_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/tunnel"
	"github.com/PierreOliveira1/pandora/internal/webhook"
)

type fakeTunnel struct {
	base          string
	exposeErr     error
	status        tunnel.Status
	statusErr     error
	unexposeErr   error
	exposePort    int
	exposePath    string
	checkPort     int
	checkPath     string
	unexposedPath string
	exposeCalls   int
	checkCalls    int
	unexposeCalls int
}

func (f *fakeTunnel) Name() string {
	return "fake"
}

func (f *fakeTunnel) Expose(ctx context.Context, port int, path string) (string, error) {
	f.exposeCalls++
	f.exposePort = port
	f.exposePath = path
	if f.exposeErr != nil {
		return "", f.exposeErr
	}
	return f.base, nil
}

func (f *fakeTunnel) Check(ctx context.Context, port int, path string) (tunnel.Status, error) {
	f.checkCalls++
	f.checkPort = port
	f.checkPath = path
	if f.statusErr != nil {
		return tunnel.Status{}, f.statusErr
	}
	return f.status, nil
}

func (f *fakeTunnel) Unexpose(ctx context.Context, path string) error {
	f.unexposeCalls++
	f.unexposedPath = path
	return f.unexposeErr
}

func addWebhookAccount(t *testing.T, f *fixture, service, account string, hook *config.Webhook) {
	t.Helper()
	err := f.store.Update(func(cfg *config.Config) error {
		if cfg.Services == nil {
			cfg.Services = map[string]*config.Service{}
		}
		svc := cfg.Services[service]
		if svc == nil {
			svc = &config.Service{Kind: config.KindCommand}
			cfg.Services[service] = svc
		}
		if svc.Accounts == nil {
			svc.Accounts = map[string]*config.Account{}
		}
		acc := svc.Accounts[account]
		if acc == nil {
			acc = &config.Account{}
			svc.Accounts[account] = acc
		}
		acc.Webhook = hook
		if svc.DefaultAccount == "" {
			svc.DefaultAccount = account
		}
		return nil
	})
	if err != nil {
		t.Fatalf("adding webhook account: %v", err)
	}
}

func setTestSecret(t *testing.T, f *fixture, key, value string) {
	t.Helper()
	if err := f.secrets.Set(key, value); err != nil {
		t.Fatalf("setting secret: %v", err)
	}
}

func coreConfig(t *testing.T, f *fixture) *config.Config {
	t.Helper()
	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	return cfg
}

func tunnelCore(t *testing.T, f *fixture, provider tunnel.Provider) *core.Core {
	t.Helper()
	app := core.New(core.Options{
		Store:      f.store,
		Secrets:    f.secrets,
		Openers:    f.openers,
		Version:    "test",
		InstallEnv: f.installEnv,
		Tunnel: func(config.WebhookSettings) tunnel.Provider {
			return provider
		},
	})
	t.Cleanup(func() {
		_ = app.Close()
	})
	return app
}

func portFromURL(t *testing.T, raw string) int {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parsing URL port: %v", err)
	}
	return port
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("closing the free port listener: %v", err)
	}
	return port
}

func TestWebhookLookupOnlyFindsTheMatchingIDAndReturnsItsSecrets(t *testing.T) {
	f := newFixture(t)
	hook := &config.Webhook{
		ID:              "hook-1",
		Style:           "hmac",
		Challenge:       "hub",
		SignatureHeader: "X-Signature",
		SignaturePrefix: "v1=",
		Encoding:        "base64",
		TokenHeader:     "X-Token",
		TokenQuery:      "token",
		TopicHeader:     "X-Topic",
		TopicFields:     []string{"entry.0.field"},
		IDHeader:        "X-Delivery",
	}
	addWebhookAccount(t, f, "instagram", "trono", hook)
	setTestSecret(t, f, secrets.WebhookSigningKey("instagram", "trono"), "app-secret")
	setTestSecret(t, f, secrets.WebhookVerifyTokenKey("instagram", "trono"), "verify-token")
	ctx := context.Background()

	if _, _, found, err := f.core.WebhookLookup(ctx, "instagram", "trono", "wrong"); err != nil || found {
		t.Fatalf("lookup with the wrong id = found %v, error %v", found, err)
	}
	if _, _, found, err := f.core.WebhookLookup(ctx, "missing", "trono", "hook-1"); err != nil || found {
		t.Fatalf("lookup with a missing service = found %v, error %v", found, err)
	}
	if _, _, found, err := f.core.WebhookLookup(ctx, "instagram", "missing", "hook-1"); err != nil || found {
		t.Fatalf("lookup with a missing account = found %v, error %v", found, err)
	}

	got, gotSecrets, found, err := f.core.WebhookLookup(ctx, "instagram", "trono", "hook-1")
	if err != nil {
		t.Fatalf("WebhookLookup() error = %v", err)
	}
	if !found {
		t.Fatal("WebhookLookup() did not find the hook")
	}
	if got.Service != "instagram" || got.Account != "trono" || got.ID != "hook-1" {
		t.Fatalf("Hook ref = %s/%s/%s", got.Service, got.Account, got.ID)
	}
	if got.Verify.Style != hook.Style || got.Verify.Challenge != hook.Challenge {
		t.Fatalf("Hook verification = %+v", got.Verify)
	}
	if got.Verify.SignatureHeader != hook.SignatureHeader || got.Verify.SignaturePrefix != hook.SignaturePrefix || got.Verify.Encoding != hook.Encoding {
		t.Fatalf("Hook signature settings = %+v", got.Verify)
	}
	if got.Verify.TokenHeader != hook.TokenHeader || got.Verify.TokenQuery != hook.TokenQuery {
		t.Fatalf("Hook token settings = %+v", got.Verify)
	}
	if got.TopicHeader != hook.TopicHeader || got.IDHeader != hook.IDHeader {
		t.Fatalf("Hook topic/id headers = %q %q", got.TopicHeader, got.IDHeader)
	}
	if len(got.TopicFields) != 1 || got.TopicFields[0] != "entry.0.field" {
		t.Fatalf("Hook topic fields = %v", got.TopicFields)
	}
	if gotSecrets.Signing != "app-secret" {
		t.Fatal("WebhookLookup() did not return the signing secret")
	}
	if gotSecrets.VerifyToken != "verify-token" {
		t.Fatal("WebhookLookup() did not return the verify token")
	}
}

func TestTheWebhookPortIsChosenOnceAndKept(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	first, err := f.core.EnsureWebhookPort(ctx)
	if err != nil {
		t.Fatalf("EnsureWebhookPort() error = %v", err)
	}
	if first <= 0 {
		t.Fatalf("EnsureWebhookPort() = %d, want a positive port", first)
	}
	second, err := f.core.EnsureWebhookPort(ctx)
	if err != nil {
		t.Fatalf("second EnsureWebhookPort() error = %v", err)
	}
	if second != first {
		t.Fatalf("second EnsureWebhookPort() = %d, want %d", second, first)
	}
	cfg := coreConfig(t, f)
	if cfg.Webhooks == nil || cfg.Webhooks.Port != first {
		t.Fatalf("saved webhook settings = %+v", cfg.Webhooks)
	}
	active, port, err := f.core.WebhooksActive()
	if err != nil {
		t.Fatalf("WebhooksActive() error = %v", err)
	}
	if active || port != first {
		t.Fatalf("WebhooksActive() = %v, %d, want false and %d", active, port, first)
	}
	addWebhookAccount(t, f, "one", "local", &config.Webhook{ID: "hook-1", Style: "hmac"})
	active, port, err = f.core.WebhooksActive()
	if err != nil {
		t.Fatalf("WebhooksActive() error = %v", err)
	}
	if !active || port != first {
		t.Fatalf("WebhooksActive() = %v, %d, want true and %d", active, port, first)
	}
}

func TestWebhooksListsThePublicAndLocalURLs(t *testing.T) {
	f := newFixture(t)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: 4321, PublicURL: "https://public.example/pandora"}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}
	addWebhookAccount(t, f, "instagram", "trono", &config.Webhook{ID: "hook-1", Style: "hmac", Challenge: "hub"})
	addWebhookAccount(t, f, "github", "work", &config.Webhook{ID: "hook-2", Style: "token"})
	addWebhookAccount(t, f, "github", "other", nil)
	ctx := context.Background()
	if _, _, err := f.core.Events().Append(ctx, events.Event{
		Service: "instagram",
		Account: "trono",
		Payload: json.RawMessage(`{}`),
	}, ""); err != nil {
		t.Fatalf("appending event: %v", err)
	}

	infos, err := f.core.Webhooks(ctx)
	if err != nil {
		t.Fatalf("Webhooks() error = %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("Webhooks() returned %d hooks, want 2", len(infos))
	}
	if infos[0].Ref != "github/work" || infos[1].Ref != "instagram/trono" {
		t.Fatalf("Webhooks() refs = %q, %q", infos[0].Ref, infos[1].Ref)
	}
	info := infos[1]
	if info.Style != "hmac" || info.Challenge != "hub" {
		t.Fatalf("instagram hook = %+v", info)
	}
	wantURL := "https://public.example/pandora/hooks/instagram/trono/hook-1"
	if info.URL != wantURL {
		t.Fatalf("URL = %q, want %q", info.URL, wantURL)
	}
	wantLocal := "http://127.0.0.1:4321/hooks/instagram/trono/hook-1"
	if info.LocalURL != wantLocal {
		t.Fatalf("LocalURL = %q, want %q", info.LocalURL, wantLocal)
	}
	if info.LastEventAt == nil {
		t.Fatal("LastEventAt is nil after a stored event")
	}
	if infos[0].LastEventAt != nil {
		t.Fatal("github/work has a LastEventAt without events")
	}
}

func TestUsingATunnelSavesThePublicURL(t *testing.T) {
	f := newFixture(t)
	fake := &fakeTunnel{base: "https://public.example/pandora"}
	app := tunnelCore(t, f, fake)
	ctx := context.Background()

	info, err := app.UseTunnel(ctx, "tailscale", "")
	if err != nil {
		t.Fatalf("UseTunnel() error = %v", err)
	}
	if !info.Exposed || info.Provider != "tailscale" || info.PublicURL != fake.base {
		t.Fatalf("UseTunnel() = %+v", info)
	}
	if info.Port <= 0 {
		t.Fatalf("UseTunnel() port = %d, want a positive port", info.Port)
	}
	if fake.exposeCalls != 1 || fake.exposePort != info.Port || fake.exposePath != core.TunnelPath {
		t.Fatalf("Expose() calls = %d, port %d, path %q", fake.exposeCalls, fake.exposePort, fake.exposePath)
	}
	cfg := coreConfig(t, f)
	if cfg.Webhooks == nil || cfg.Webhooks.Tunnel != "tailscale" || cfg.Webhooks.PublicURL != fake.base || cfg.Webhooks.Port != info.Port {
		t.Fatalf("saved webhook settings = %+v", cfg.Webhooks)
	}

	if _, err := app.UseTunnel(ctx, "ngrok", ""); err == nil || err.Error() != "unknown tunnel ngrok: use tailscale or url" {
		t.Fatalf("unknown tunnel error = %v", err)
	}
	if _, err := app.UseTunnel(ctx, "url", ""); err == nil {
		t.Fatal("UseTunnel() accepted url without a base")
	}
}

func TestTheURLTunnelUsesTheConfiguredBase(t *testing.T) {
	f := newFixture(t)

	info, err := f.core.UseTunnel(context.Background(), "url", "https://hooks.example.com/")
	if err != nil {
		t.Fatalf("UseTunnel() error = %v", err)
	}
	if !info.Exposed || info.Provider != "url" || info.PublicURL != "https://hooks.example.com" {
		t.Fatalf("UseTunnel() = %+v", info)
	}
	cfg := coreConfig(t, f)
	if cfg.Webhooks == nil || cfg.Webhooks.Tunnel != "url" || cfg.Webhooks.PublicURL != "https://hooks.example.com" {
		t.Fatalf("saved webhook settings = %+v", cfg.Webhooks)
	}
}

func TestTunnelStatusReportsWhenTheFunnelIsNotServingThePath(t *testing.T) {
	f := newFixture(t)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{
			Port:      4321,
			Tunnel:    "tailscale",
			PublicURL: "https://public.example/pandora",
		}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}
	fake := &fakeTunnel{status: tunnel.Status{
		Exposed:    false,
		PublicBase: "https://public.example/pandora",
		Problem:    "Tailscale Funnel is not serving /pandora: run pandora tunnel use tailscale",
	}}
	app := tunnelCore(t, f, fake)

	info, err := app.TunnelStatus(context.Background())
	if err != nil {
		t.Fatalf("TunnelStatus() error = %v", err)
	}
	if info.Exposed || info.Provider != "tailscale" || info.Port != 4321 || info.PublicURL != "https://public.example/pandora" {
		t.Fatalf("TunnelStatus() = %+v", info)
	}
	if info.Problem != "Tailscale Funnel is not serving /pandora: run pandora tunnel use tailscale" {
		t.Fatalf("Problem = %q", info.Problem)
	}
	if fake.checkCalls != 1 || fake.checkPort != 4321 || fake.checkPath != core.TunnelPath {
		t.Fatalf("Check() calls = %d, port %d, path %q", fake.checkCalls, fake.checkPort, fake.checkPath)
	}
}

func TestTunnelStatusExplainsHowToStartWhenNoTunnelIsConfigured(t *testing.T) {
	f := newFixture(t)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: 4321}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}
	fake := &fakeTunnel{}
	app := tunnelCore(t, f, fake)

	info, err := app.TunnelStatus(context.Background())
	if err != nil {
		t.Fatalf("TunnelStatus() error = %v", err)
	}
	if info.Port != 4321 || info.Exposed || info.Problem != "no tunnel: run pandora tunnel use tailscale" {
		t.Fatalf("TunnelStatus() = %+v", info)
	}
	if fake.checkCalls != 0 {
		t.Fatalf("Check() calls = %d, want none", fake.checkCalls)
	}
}

func TestStoppingTheTunnelKeepsTheReceiverPort(t *testing.T) {
	f := newFixture(t)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{
			Port:      4321,
			Tunnel:    "tailscale",
			PublicURL: "https://public.example/pandora",
		}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}
	fake := &fakeTunnel{}
	app := tunnelCore(t, f, fake)

	if err := app.StopTunnel(context.Background()); err != nil {
		t.Fatalf("StopTunnel() error = %v", err)
	}
	if fake.unexposeCalls != 1 || fake.unexposedPath != core.TunnelPath {
		t.Fatalf("Unexpose() calls = %d, path %q", fake.unexposeCalls, fake.unexposedPath)
	}
	cfg := coreConfig(t, f)
	if cfg.Webhooks == nil || cfg.Webhooks.Tunnel != "" || cfg.Webhooks.PublicURL != "" || cfg.Webhooks.Port != 4321 {
		t.Fatalf("saved webhook settings = %+v", cfg.Webhooks)
	}
	if err := app.StopTunnel(context.Background()); err != nil {
		t.Fatalf("second StopTunnel() error = %v", err)
	}
	if fake.unexposeCalls != 1 {
		t.Fatalf("Unexpose() calls after the second stop = %d, want 1", fake.unexposeCalls)
	}
}

func TestTestingAWebhookDeliversASignedEventToTheQueue(t *testing.T) {
	f := newFixture(t)
	addWebhookAccount(t, f, "instagram", "trono", &config.Webhook{
		ID:              "hook-1",
		Style:           "hmac",
		Challenge:       "hub",
		SignatureHeader: "X-Hub-Signature-256",
		SignaturePrefix: "sha256=",
		Encoding:        "hex",
	})
	setTestSecret(t, f, secrets.WebhookSigningKey("instagram", "trono"), "app-secret")
	setTestSecret(t, f, secrets.WebhookVerifyTokenKey("instagram", "trono"), "verify-token")

	server := httptest.NewServer(webhook.NewHandler(f.core.WebhookLookup, f.core.Events()))
	t.Cleanup(server.Close)
	port := portFromURL(t, server.URL)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: port}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}

	result, err := f.core.TestWebhook(context.Background(), "instagram/trono")
	if err != nil {
		t.Fatalf("TestWebhook() error = %v", err)
	}
	if !result.Handshake {
		t.Fatal("TestWebhook() did not complete the handshake")
	}
	wantURL := fmt.Sprintf("http://127.0.0.1:%d/hooks/instagram/trono/hook-1", port)
	if result.URL != wantURL {
		t.Fatalf("TestWebhook() URL = %q, want %q", result.URL, wantURL)
	}

	pending, err := f.core.PendingEvents(context.Background(), "", events.Filter{})
	if err != nil {
		t.Fatalf("PendingEvents() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("PendingEvents() = %d events, want 1", len(pending))
	}
	event := pending[0]
	if event.Service != "instagram" || event.Account != "trono" {
		t.Fatalf("event ref = %s/%s", event.Service, event.Account)
	}
	if !strings.Contains(string(event.Payload), `"pandora_test":true`) {
		t.Fatalf("event payload = %s", event.Payload)
	}
	if event.Headers["Content-Type"] != "application/json" {
		t.Fatalf("event Content-Type = %q", event.Headers["Content-Type"])
	}
}

func TestTestingAWebhookSignsATokenQuery(t *testing.T) {
	f := newFixture(t)
	addWebhookAccount(t, f, "one", "local", &config.Webhook{
		ID:         "hook-1",
		Style:      "token",
		TokenQuery: "token",
	})
	setTestSecret(t, f, secrets.WebhookSigningKey("one", "local"), "token-secret")
	server := httptest.NewServer(webhook.NewHandler(f.core.WebhookLookup, f.core.Events()))
	t.Cleanup(server.Close)
	port := portFromURL(t, server.URL)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: port}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}

	result, err := f.core.TestWebhook(context.Background(), "one/local")
	if err != nil {
		t.Fatalf("TestWebhook() error = %v", err)
	}
	if result.Handshake {
		t.Fatal("TestWebhook() ran a handshake without the hub challenge")
	}
	pending, err := f.core.PendingEvents(context.Background(), "", events.Filter{})
	if err != nil {
		t.Fatalf("PendingEvents() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("PendingEvents() = %d events, want 1", len(pending))
	}
}

func TestTestingAWebhookWithoutAReceiverExplainsHowToStartTheDaemon(t *testing.T) {
	f := newFixture(t)
	addWebhookAccount(t, f, "instagram", "trono", &config.Webhook{ID: "hook-1", Style: "hmac"})
	setTestSecret(t, f, secrets.WebhookSigningKey("instagram", "trono"), "app-secret")
	port := freeTCPPort(t)
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: port}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}

	_, err := f.core.TestWebhook(context.Background(), "instagram/trono")
	if err == nil {
		t.Fatal("TestWebhook() succeeded without a receiver")
	}
	want := "the webhook receiver is not running: run pandora daemon start"
	if err.Error() != want {
		t.Fatalf("TestWebhook() error = %q, want %q", err, want)
	}
}

func TestWebhookVerifyTokenReturnsTheTokenOnlyForTheHubChallenge(t *testing.T) {
	f := newFixture(t)
	addWebhookAccount(t, f, "instagram", "trono", &config.Webhook{ID: "hook-1", Style: "hmac", Challenge: "hub"})
	setTestSecret(t, f, secrets.WebhookVerifyTokenKey("instagram", "trono"), "verify-token")
	ctx := context.Background()

	token, err := f.core.WebhookVerifyToken(ctx, "instagram/trono")
	if err != nil {
		t.Fatalf("WebhookVerifyToken() error = %v", err)
	}
	if token != "verify-token" {
		t.Fatal("WebhookVerifyToken() did not return the saved token")
	}

	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Services["instagram"].Accounts["trono"].Webhook.Challenge = ""
		return nil
	}); err != nil {
		t.Fatalf("changing the challenge: %v", err)
	}
	token, err = f.core.WebhookVerifyToken(ctx, "instagram/trono")
	if err != nil {
		t.Fatalf("WebhookVerifyToken() without hub error = %v", err)
	}
	if token != "" {
		t.Fatal("WebhookVerifyToken() returned a token without the hub challenge")
	}

	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Services["instagram"].Accounts["trono"].Webhook = nil
		return nil
	}); err != nil {
		t.Fatalf("removing the webhook: %v", err)
	}
	_, err = f.core.WebhookVerifyToken(ctx, "instagram/trono")
	want := "instagram/trono has no webhook: add one with pandora webhook add instagram/trono"
	if err == nil || err.Error() != want {
		t.Fatalf("WebhookVerifyToken() error = %v, want %q", err, want)
	}
}

func TestAcknowledgingSelectedEventsLeavesTheRestPending(t *testing.T) {
	f := newFixture(t)
	queue := f.core.Events()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, _, err := queue.Append(ctx, events.Event{
			Service: "one",
			Account: "local",
			Payload: json.RawMessage(`{}`),
		}, ""); err != nil {
			t.Fatalf("appending event: %v", err)
		}
	}
	pending, err := f.core.PendingEvents(ctx, "ai", events.Filter{})
	if err != nil {
		t.Fatalf("PendingEvents() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("PendingEvents() = %d events, want 2", len(pending))
	}
	if err := f.core.AckEvents(ctx, "ai", []int64{pending[0].Seq}); err != nil {
		t.Fatalf("AckEvents() error = %v", err)
	}
	remaining, err := f.core.PendingEvents(ctx, "ai", events.Filter{})
	if err != nil {
		t.Fatalf("PendingEvents() after AckEvents() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Seq != pending[1].Seq {
		t.Fatalf("PendingEvents() after AckEvents() = %+v, want the second event", remaining)
	}
}

func TestAcknowledgingAllEventsOfOneFilterKeepsOtherEventsPending(t *testing.T) {
	f := newFixture(t)
	queue := f.core.Events()
	ctx := context.Background()
	for _, service := range []string{"alpha", "alpha", "beta"} {
		if _, _, err := queue.Append(ctx, events.Event{
			Service: service,
			Account: "local",
			Payload: json.RawMessage(`{}`),
		}, ""); err != nil {
			t.Fatalf("appending event: %v", err)
		}
	}

	count, err := f.core.AckAllEvents(ctx, "", events.Filter{Service: "alpha"})
	if err != nil {
		t.Fatalf("AckAllEvents() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("AckAllEvents() = %d, want 2", count)
	}
	pending, err := f.core.PendingEvents(ctx, "", events.Filter{})
	if err != nil {
		t.Fatalf("PendingEvents() error = %v", err)
	}
	if len(pending) != 1 || pending[0].Service != "beta" {
		t.Fatalf("PendingEvents() = %+v, want the beta event", pending)
	}
	alpha, err := f.core.PendingEvents(ctx, "", events.Filter{Service: "alpha"})
	if err != nil {
		t.Fatalf("PendingEvents(alpha) error = %v", err)
	}
	if len(alpha) != 0 {
		t.Fatalf("PendingEvents(alpha) = %+v, want none", alpha)
	}
}
