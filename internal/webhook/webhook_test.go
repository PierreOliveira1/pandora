package webhook_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/webhook"
)

type fixture struct {
	server    *httptest.Server
	queue     *events.Queue
	hook      webhook.Hook
	secrets   webhook.Secrets
	found     bool
	lookupErr error
}

func newFixture(t *testing.T, hook webhook.Hook, secrets webhook.Secrets) *fixture {
	t.Helper()
	f := &fixture{
		queue:   events.NewQueue(t.TempDir(), time.Now),
		hook:    hook,
		secrets: secrets,
		found:   true,
	}
	lookup := func(_ context.Context, service, account, id string) (webhook.Hook, webhook.Secrets, bool, error) {
		if f.lookupErr != nil {
			return webhook.Hook{}, webhook.Secrets{}, false, f.lookupErr
		}
		if !f.found {
			return webhook.Hook{}, webhook.Secrets{}, false, nil
		}
		if service != hook.Service || account != hook.Account || id != hook.ID {
			return webhook.Hook{}, webhook.Secrets{}, false, nil
		}
		return hook, secrets, true, nil
	}
	f.server = httptest.NewServer(webhook.NewHandler(lookup, f.queue))
	t.Cleanup(f.server.Close)
	return f
}

type response struct {
	status int
	body   string
	header http.Header
}

func (f *fixture) do(t *testing.T, method, path string, body []byte, headers map[string]string) response {
	t.Helper()
	request, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	resp, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return response{status: resp.StatusCode, body: string(data), header: resp.Header}
}

func hookPath(hook webhook.Hook, prefixed bool) string {
	prefix := ""
	if prefixed {
		prefix = "/pandora"
	}
	return prefix + "/hooks/" + hook.Service + "/" + hook.Account + "/" + hook.ID
}

func storedEvents(t *testing.T, f *fixture) []events.Event {
	t.Helper()
	got, err := f.queue.Pending(context.Background(), "test", events.Filter{})
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	return got
}

func hmacSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestTheHubHandshakeOnlyAcceptsTheConfiguredVerifyToken(t *testing.T) {
	hook := webhook.Hook{
		Service: "instagram",
		Account: "trono",
		ID:      "hook-1",
		Verify:  webhook.Verification{Style: "hmac", Challenge: "hub"},
	}
	f := newFixture(t, hook, webhook.Secrets{Signing: "secret", VerifyToken: "verify-token"})

	query := url.Values{
		"hub.mode":         {"subscribe"},
		"hub.verify_token": {"verify-token"},
		"hub.challenge":    {"challenge-123"},
	}
	resp := f.do(t, http.MethodGet, hookPath(hook, false)+"?"+query.Encode(), nil, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", resp.status, resp.body)
	}
	if resp.body != "challenge-123" {
		t.Errorf("GET body = %q, want the challenge", resp.body)
	}
	if contentType := resp.header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}

	query.Set("hub.verify_token", "wrong")
	if resp := f.do(t, http.MethodGet, hookPath(hook, false)+"?"+query.Encode(), nil, nil); resp.status != http.StatusForbidden {
		t.Errorf("GET with wrong token status = %d, want 403", resp.status)
	}

	query.Set("hub.verify_token", "verify-token")
	query.Set("hub.mode", "unsubscribe")
	if resp := f.do(t, http.MethodGet, hookPath(hook, false)+"?"+query.Encode(), nil, nil); resp.status != http.StatusForbidden {
		t.Errorf("GET with wrong mode status = %d, want 403", resp.status)
	}
}

func TestAHookWithoutTheHubChallengeDoesNotAnswerGET(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "hmac"}}
	f := newFixture(t, hook, webhook.Secrets{Signing: "secret"})

	if resp := f.do(t, http.MethodGet, hookPath(hook, false), nil, nil); resp.status != http.StatusNotFound {
		t.Errorf("GET status = %d, want 404", resp.status)
	}
}

func TestASignedDeliveryIsStoredWithItsTopicAndPayload(t *testing.T) {
	hook := webhook.Hook{
		Service:     "instagram",
		Account:     "trono",
		ID:          "hook-1",
		Verify:      webhook.Verification{Style: "hmac"},
		TopicHeader: "X-Topic",
		IDHeader:    "X-Delivery",
	}
	f := newFixture(t, hook, webhook.Secrets{Signing: "app-secret"})

	body := []byte(`{"entry":[{"changes":[{"field":"comments","value":{"id":"1"}}]}]}`)
	headers := map[string]string{
		"Content-Type":        "application/json",
		"User-Agent":          "pandora-test",
		"X-Topic":             "comments",
		"X-Delivery":          "delivery-1",
		"X-Hub-Signature-256": hmacSignature("app-secret", body),
	}
	resp := f.do(t, http.MethodPost, hookPath(hook, false), body, headers)
	if resp.status != http.StatusOK || resp.body != "ok" {
		t.Fatalf("POST = %d %q, want 200 ok", resp.status, resp.body)
	}

	stored := storedEvents(t, f)
	if len(stored) != 1 {
		t.Fatalf("stored events = %d, want 1", len(stored))
	}
	ev := stored[0]
	if ev.Service != hook.Service || ev.Account != hook.Account {
		t.Errorf("event service/account = %s/%s, want %s/%s", ev.Service, ev.Account, hook.Service, hook.Account)
	}
	if ev.Topic != "comments" {
		t.Errorf("event topic = %q, want comments", ev.Topic)
	}
	if string(ev.Payload) != string(body) {
		t.Errorf("event payload = %s, want %s", ev.Payload, body)
	}
	if ev.ID == "" || ev.ReceivedAt.IsZero() || ev.Seq != 1 {
		t.Errorf("event id/received_at/seq = %q/%v/%d, want a generated event", ev.ID, ev.ReceivedAt, ev.Seq)
	}
	for _, name := range []string{"Content-Type", "User-Agent", "X-Topic", "X-Delivery"} {
		if ev.Headers[name] != headers[name] {
			t.Errorf("event header %s = %q, want %q", name, ev.Headers[name], headers[name])
		}
	}
	if _, ok := ev.Headers["X-Hub-Signature-256"]; ok {
		t.Error("event stored the signature header")
	}

	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, headers); resp.status != http.StatusOK {
		t.Errorf("repeated POST status = %d, want 200", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 1 {
		t.Errorf("stored events after a duplicate = %d, want 1", len(stored))
	}
}

func TestAWrongSignatureIsRejectedAndNothingIsStored(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "hmac"}}
	f := newFixture(t, hook, webhook.Secrets{Signing: "right-secret"})
	body := []byte(`{"a":1}`)

	headers := map[string]string{"X-Hub-Signature-256": hmacSignature("wrong-secret", body)}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, headers); resp.status != http.StatusUnauthorized {
		t.Fatalf("POST status = %d, want 401", resp.status)
	}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, nil); resp.status != http.StatusUnauthorized {
		t.Fatalf("POST without a signature status = %d, want 401", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 0 {
		t.Errorf("stored events = %d, want none", len(stored))
	}
}

func TestAHookWithoutASigningSecretRejectsEveryDelivery(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "hmac"}}
	f := newFixture(t, hook, webhook.Secrets{})
	body := []byte(`{"a":1}`)

	headers := map[string]string{"X-Hub-Signature-256": hmacSignature("", body)}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, headers); resp.status != http.StatusUnauthorized {
		t.Fatalf("POST status = %d, want 401", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 0 {
		t.Errorf("stored events = %d, want none", len(stored))
	}
}

func TestTheHubHandshakeFailsWhenNoVerifyTokenIsConfigured(t *testing.T) {
	hook := webhook.Hook{
		Service: "instagram",
		Account: "trono",
		ID:      "hook-1",
		Verify:  webhook.Verification{Style: "hmac", Challenge: "hub"},
	}
	f := newFixture(t, hook, webhook.Secrets{Signing: "secret"})
	query := url.Values{
		"hub.mode":      {"subscribe"},
		"hub.challenge": {"challenge-123"},
	}

	if resp := f.do(t, http.MethodGet, hookPath(hook, false)+"?"+query.Encode(), nil, nil); resp.status != http.StatusForbidden {
		t.Fatalf("GET status = %d, want 403", resp.status)
	}
}

func TestTheSameBodyDeliveredToTwoAccountsIsStoredForBoth(t *testing.T) {
	queue := events.NewQueue(t.TempDir(), time.Now)
	hook := webhook.Hook{Service: "one", Account: "a", ID: "hook-1", Verify: webhook.Verification{Style: "none"}}
	lookup := func(_ context.Context, service, account, id string) (webhook.Hook, webhook.Secrets, bool, error) {
		if service != "one" || id != "hook-1" || (account != "a" && account != "b") {
			return webhook.Hook{}, webhook.Secrets{}, false, nil
		}
		hook.Account = account
		return hook, webhook.Secrets{}, true, nil
	}
	server := httptest.NewServer(webhook.NewHandler(lookup, queue))
	t.Cleanup(server.Close)

	body := []byte(`{"same":true}`)
	for _, account := range []string{"a", "b"} {
		resp, err := http.Post(server.URL+"/hooks/one/"+account+"/hook-1", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST to account %s error = %v", account, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST to account %s status = %d, want 200", account, resp.StatusCode)
		}
	}

	stored, err := queue.Pending(context.Background(), "test", events.Filter{})
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored events = %d, want 2", len(stored))
	}
	if stored[0].Account != "a" || stored[1].Account != "b" {
		t.Errorf("stored accounts = %s and %s, want a and b", stored[0].Account, stored[1].Account)
	}
}

func TestABodyLargerThanOneMegabyteIsRejected(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "none"}}
	f := newFixture(t, hook, webhook.Secrets{})
	body := bytes.Repeat([]byte("a"), 1<<20+1)

	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, nil); resp.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST status = %d, want 413", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 0 {
		t.Errorf("stored events = %d, want none", len(stored))
	}
}

func TestAnUnknownWebhookIDReturnsNotFound(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "none"}}
	f := newFixture(t, hook, webhook.Secrets{})
	f.found = false

	resp := f.do(t, http.MethodPost, hookPath(hook, false), []byte(`{}`), nil)
	missing := f.do(t, http.MethodGet, "/unknown", nil, nil)
	if resp.status != http.StatusNotFound {
		t.Fatalf("POST status = %d, want 404", resp.status)
	}
	if resp.body != missing.body {
		t.Errorf("unknown hook body = %q, want the same body as a missing route %q", resp.body, missing.body)
	}
	if stored := storedEvents(t, f); len(stored) != 0 {
		t.Errorf("stored events = %d, want none", len(stored))
	}
}

func TestALookupFailureReturnsAServerError(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "none"}}
	f := newFixture(t, hook, webhook.Secrets{})
	f.lookupErr = errors.New("boom")

	if resp := f.do(t, http.MethodPost, hookPath(hook, false), []byte(`{}`), nil); resp.status != http.StatusInternalServerError {
		t.Fatalf("POST status = %d, want 500", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 0 {
		t.Errorf("stored events = %d, want none", len(stored))
	}
}

func TestTheTopicComesFromTheHeaderTheFirstFieldOrTheFieldName(t *testing.T) {
	tests := []struct {
		name    string
		hook    webhook.Hook
		body    string
		headers map[string]string
		want    string
	}{
		{
			name:    "header",
			hook:    webhook.Hook{TopicHeader: "X-Topic"},
			body:    `{}`,
			headers: map[string]string{"X-Topic": "from-header"},
			want:    "from-header",
		},
		{
			name: "first field that exists",
			hook: webhook.Hook{TopicFields: []string{"missing.path", "entry.0.field", "entry.0.other"}},
			body: `{"entry":[{"field":"from-field","other":"ignored"}]}`,
			want: "from-field",
		},
		{
			name: "field whose value is not text",
			hook: webhook.Hook{TopicFields: []string{"entry.0.messaging"}},
			body: `{"entry":[{"messaging":[{"sender":"x"}]}]}`,
			want: "messaging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := webhook.Hook{
				Service:     "one",
				Account:     "local",
				ID:          "hook-1",
				Verify:      webhook.Verification{Style: "none"},
				TopicHeader: tt.hook.TopicHeader,
				TopicFields: tt.hook.TopicFields,
			}
			f := newFixture(t, hook, webhook.Secrets{})
			if resp := f.do(t, http.MethodPost, hookPath(hook, false), []byte(tt.body), tt.headers); resp.status != http.StatusOK {
				t.Fatalf("POST status = %d, want 200", resp.status)
			}
			stored := storedEvents(t, f)
			if len(stored) != 1 {
				t.Fatalf("stored events = %d, want 1", len(stored))
			}
			if stored[0].Topic != tt.want {
				t.Errorf("topic = %q, want %q", stored[0].Topic, tt.want)
			}
		})
	}
}

func TestTokenVerificationAcceptsTheHeaderOrTheQuery(t *testing.T) {
	hook := webhook.Hook{
		Service: "one",
		Account: "local",
		ID:      "hook-1",
		Verify: webhook.Verification{
			Style:       "token",
			TokenHeader: "X-Token",
			TokenQuery:  "token",
		},
	}
	f := newFixture(t, hook, webhook.Secrets{Signing: "tok"})
	body := []byte(`{"n":1}`)

	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, map[string]string{"X-Token": "tok"}); resp.status != http.StatusOK {
		t.Fatalf("POST with token header status = %d, want 200", resp.status)
	}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false)+"?token=tok", []byte(`{"n":2}`), nil); resp.status != http.StatusOK {
		t.Fatalf("POST with token query status = %d, want 200", resp.status)
	}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false)+"?token=wrong", body, nil); resp.status != http.StatusUnauthorized {
		t.Fatalf("POST with wrong token status = %d, want 401", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 2 {
		t.Errorf("stored events = %d, want 2", len(stored))
	}
}

func TestTheNoneStyleAcceptsAnUnsignedDelivery(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "none"}}
	f := newFixture(t, hook, webhook.Secrets{})

	if resp := f.do(t, http.MethodPost, hookPath(hook, false), []byte(`{}`), nil); resp.status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 1 {
		t.Errorf("stored events = %d, want 1", len(stored))
	}
}

func TestSignProducesASignatureTheReceiverAccepts(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "hmac"}}
	f := newFixture(t, hook, webhook.Secrets{Signing: "signing-key"})
	body := []byte(`{"hello":"world"}`)

	header, value := webhook.Sign(hook.Verify, "signing-key", body)
	if header != "X-Hub-Signature-256" {
		t.Errorf("Sign() header = %q, want X-Hub-Signature-256", header)
	}
	if !strings.HasPrefix(value, "sha256=") {
		t.Errorf("Sign() value = %q, want the sha256= prefix", value)
	}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, map[string]string{header: value}); resp.status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 1 {
		t.Errorf("stored events = %d, want 1", len(stored))
	}
}

func TestSignSupportsBase64AndACustomPrefix(t *testing.T) {
	verify := webhook.Verification{Style: "hmac", SignatureHeader: "X-Signature", SignaturePrefix: "v1=", Encoding: "base64"}
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: verify}
	f := newFixture(t, hook, webhook.Secrets{Signing: "signing-key"})
	body := []byte(`{"hello":"world"}`)

	header, value := webhook.Sign(verify, "signing-key", body)
	if header != "X-Signature" || !strings.HasPrefix(value, "v1=") {
		t.Fatalf("Sign() = %q %q, want a X-Signature header with the v1= prefix", header, value)
	}
	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, map[string]string{header: value}); resp.status != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 1 {
		t.Errorf("stored events = %d, want 1", len(stored))
	}
}

func TestTheSameHookAnswersWithAndWithoutThePandoraPrefix(t *testing.T) {
	hook := webhook.Hook{Service: "one", Account: "local", ID: "hook-1", Verify: webhook.Verification{Style: "none"}}
	f := newFixture(t, hook, webhook.Secrets{})
	body := []byte(`{"n":1}`)

	if resp := f.do(t, http.MethodPost, hookPath(hook, false), body, nil); resp.status != http.StatusOK {
		t.Fatalf("POST without prefix status = %d, want 200", resp.status)
	}
	if resp := f.do(t, http.MethodPost, hookPath(hook, true), []byte(`{"n":2}`), nil); resp.status != http.StatusOK {
		t.Fatalf("POST with prefix status = %d, want 200", resp.status)
	}
	if stored := storedEvents(t, f); len(stored) != 2 {
		t.Errorf("stored events = %d, want 2", len(stored))
	}
}
