package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/webhook"
)

type webhookDaemonFixture struct {
	paths   config.Paths
	store   *config.Store
	secrets secrets.Store
	core    *core.Core
	server  *Server
	client  *Client
	cancel  context.CancelFunc
	done    chan struct{}
	errMu   sync.Mutex
	err     error
}

func newWebhookDaemonFixture(t *testing.T, idle time.Duration) *webhookDaemonFixture {
	t.Helper()
	root, err := os.MkdirTemp("", "pdw")
	if err != nil {
		t.Fatalf("creating test directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
	paths := config.Paths{
		ConfigDir: filepath.Join(root, "c"),
		CacheDir:  filepath.Join(root, "k"),
		StateDir:  root,
	}
	store := config.NewStore(paths)
	sec := secrets.NewMemoryStore()
	app := core.New(core.Options{
		Store:   store,
		Secrets: sec,
		Version: "test",
	})
	server := NewServer(app, "test", idle)
	server.reloadEvery = 20 * time.Millisecond
	server.readerBackoff = []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond}
	f := &webhookDaemonFixture{
		paths:   paths,
		store:   store,
		secrets: sec,
		core:    app,
		server:  server,
		done:    make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	t.Cleanup(func() {
		f.close(t)
	})
	go func() {
		err := server.Serve(ctx, paths.DaemonSocket())
		f.errMu.Lock()
		f.err = err
		f.errMu.Unlock()
		close(f.done)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		client, err := Dial(ctx, paths.DaemonSocket())
		if err == nil {
			f.client = client
			break
		}
		select {
		case <-f.done:
			t.Fatalf("daemon stopped before starting: %v", f.serveError())
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon did not start: %v", err)
		}
	}
	return f
}

func (f *webhookDaemonFixture) close(t *testing.T) {
	t.Helper()
	if f.client != nil {
		_ = f.client.Shutdown(context.Background())
	}
	f.cancel()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Errorf("daemon did not stop")
	}
	_ = f.core.Close()
}

func (f *webhookDaemonFixture) serveError() error {
	f.errMu.Lock()
	defer f.errMu.Unlock()
	return f.err
}

func (f *webhookDaemonFixture) addWebhook(t *testing.T, service, account string, hook *config.Webhook, signing, verifyToken string) {
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
		t.Fatalf("adding webhook: %v", err)
	}
	if signing != "" {
		if err := f.secrets.Set(secrets.WebhookSigningKey(service, account), signing); err != nil {
			t.Fatalf("setting signing secret: %v", err)
		}
	}
	if verifyToken != "" {
		if err := f.secrets.Set(secrets.WebhookVerifyTokenKey(service, account), verifyToken); err != nil {
			t.Fatalf("setting verify token: %v", err)
		}
	}
}

func (f *webhookDaemonFixture) removeWebhook(t *testing.T, service, account string) {
	t.Helper()
	err := f.store.Update(func(cfg *config.Config) error {
		svc := cfg.Services[service]
		if svc == nil {
			return nil
		}
		acc := svc.Accounts[account]
		if acc == nil {
			return nil
		}
		acc.Webhook = nil
		return nil
	})
	if err != nil {
		t.Fatalf("removing webhook: %v", err)
	}
}

func (f *webhookDaemonFixture) addReader(t *testing.T, reader config.EventReader) {
	t.Helper()
	err := f.store.Update(func(cfg *config.Config) error {
		readers := make([]config.EventReader, 0, len(cfg.Readers)+1)
		for _, existing := range cfg.Readers {
			if existing.Name != reader.Name {
				readers = append(readers, existing)
			}
		}
		cfg.Readers = append(readers, reader)
		return nil
	})
	if err != nil {
		t.Fatalf("adding reader: %v", err)
	}
}

func (f *webhookDaemonFixture) removeReader(t *testing.T, name string) {
	t.Helper()
	err := f.store.Update(func(cfg *config.Config) error {
		readers := make([]config.EventReader, 0, len(cfg.Readers))
		for _, existing := range cfg.Readers {
			if existing.Name != name {
				readers = append(readers, existing)
			}
		}
		cfg.Readers = readers
		return nil
	})
	if err != nil {
		t.Fatalf("removing reader: %v", err)
	}
}

func (f *webhookDaemonFixture) appendEvent(t *testing.T, service, account, topic string) events.Event {
	t.Helper()
	event, _, err := f.core.Events().Append(context.Background(), events.Event{
		Service: service,
		Account: account,
		Topic:   topic,
		Payload: json.RawMessage(`{"ok":true}`),
	}, "")
	if err != nil {
		t.Fatalf("appending event: %v", err)
	}
	return event
}

func waitForWebhookCondition(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type testReaderHelperOutput struct {
	Stdin   string `json:"stdin"`
	Seq     string `json:"seq"`
	ID      string `json:"id"`
	Service string `json:"service"`
	Account string `json:"account"`
	Topic   string `json:"topic"`
	Reader  string `json:"reader"`
}

func testReaderCommand() []string {
	return []string{os.Args[0], "-test.run=TestReaderHelperProcess"}
}

func waitForTestReaderOutput(t *testing.T, path string) testReaderHelperOutput {
	t.Helper()
	var result testReaderHelperOutput
	waitForWebhookCondition(t, 10*time.Second, func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		return json.Unmarshal(data, &result) == nil
	}, "the reader command did not write its output")
	return result
}

func TestReaderHelperProcess(t *testing.T) {
	if os.Getenv("PANDORA_READER_HELPER") != "1" {
		return
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reading stdin failed")
		os.Exit(1)
	}
	attempt := 1
	if counter := os.Getenv("PANDORA_READER_HELPER_COUNTER"); counter != "" {
		data, _ := os.ReadFile(counter)
		if parsed, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			attempt = parsed + 1
		}
		_ = os.WriteFile(counter, []byte(strconv.Itoa(attempt)), 0o600)
	}
	failures, _ := strconv.Atoi(os.Getenv("PANDORA_READER_HELPER_FAILURES"))
	if attempt <= failures {
		fmt.Fprintf(os.Stderr, "attempt %d failed\n", attempt)
		os.Exit(1)
	}
	result := testReaderHelperOutput{
		Stdin:   string(input),
		Seq:     os.Getenv("PANDORA_EVENT_SEQ"),
		ID:      os.Getenv("PANDORA_EVENT_ID"),
		Service: os.Getenv("PANDORA_EVENT_SERVICE"),
		Account: os.Getenv("PANDORA_EVENT_ACCOUNT"),
		Topic:   os.Getenv("PANDORA_EVENT_TOPIC"),
		Reader:  os.Getenv("PANDORA_READER"),
	}
	data, err := json.Marshal(result)
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(os.Getenv("PANDORA_READER_HELPER_OUTPUT"), data, 0o600); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestTheDaemonReceivesASignedWebhookAndQueuesIt(t *testing.T) {
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	f.addWebhook(t, "instagram", "trono", &config.Webhook{ID: "hook-1", Style: "hmac"}, "app-secret", "")
	state, err := f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if !state.Listening || state.Address == "" {
		t.Fatalf("ReloadWebhooks() = %+v, want a listening receiver", state)
	}

	body := []byte(`{"object":"test"}`)
	header, signature := webhook.Sign(webhook.Verification{Style: "hmac"}, "app-secret", body)
	request, err := http.NewRequest(http.MethodPost, "http://"+state.Address+"/hooks/instagram/trono/hook-1", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(header, signature)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("delivery error = %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("delivery status = %d, want 200", response.StatusCode)
	}

	pending, err := f.core.PendingEvents(context.Background(), "", events.Filter{})
	if err != nil {
		t.Fatalf("PendingEvents() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("PendingEvents() = %d events, want 1", len(pending))
	}
	if pending[0].Service != "instagram" || pending[0].Account != "trono" {
		t.Fatalf("event ref = %s/%s", pending[0].Service, pending[0].Account)
	}

	response, err = client.Get("http://" + state.Address + "/v1/health")
	if err != nil {
		t.Fatalf("receiver health request error = %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("receiver /v1/health status = %d, want 404", response.StatusCode)
	}
}

func TestTheDaemonDoesNotIdleWhileAWebhookIsActive(t *testing.T) {
	f := newWebhookDaemonFixture(t, 200*time.Millisecond)
	f.addWebhook(t, "one", "local", &config.Webhook{ID: "hook-1", Style: "hmac"}, "secret", "")
	state, err := f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if !state.Listening {
		t.Fatalf("ReloadWebhooks() = %+v, want a listening receiver", state)
	}

	time.Sleep(3 * 200 * time.Millisecond)
	select {
	case <-f.done:
		t.Fatalf("daemon stopped with an active webhook: %v", f.serveError())
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := f.client.Health(ctx); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestReloadStartsAndStopsTheReceiver(t *testing.T) {
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	state, err := f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if state.Listening {
		t.Fatalf("ReloadWebhooks() = %+v, want no receiver", state)
	}

	f.addWebhook(t, "one", "local", &config.Webhook{ID: "hook-1", Style: "hmac"}, "secret", "")
	state, err = f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if !state.Listening || state.Address == "" {
		t.Fatalf("ReloadWebhooks() = %+v, want a listening receiver", state)
	}
	active, port, err := f.core.WebhooksActive()
	if err != nil {
		t.Fatalf("WebhooksActive() error = %v", err)
	}
	if !active || port == 0 {
		t.Fatalf("WebhooksActive() = %v, %d", active, port)
	}

	f.removeWebhook(t, "one", "local")
	state, err = f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if state.Listening || state.Address != "" {
		t.Fatalf("ReloadWebhooks() = %+v, want the receiver stopped", state)
	}
}

func TestReloadReportsWhenTheWebhookPortIsInUse(t *testing.T) {
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	f.addWebhook(t, "one", "local", &config.Webhook{ID: "hook-1", Style: "hmac"}, "secret", "")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupying a port: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: port}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}

	state, err := f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	want := fmt.Sprintf("port %d is in use: another program is using it", port)
	if state.Listening || state.Error != want {
		t.Fatalf("ReloadWebhooks() = %+v, want error %q", state, want)
	}
}

func TestAReaderCommandReceivesTheEventAndAcknowledgesIt(t *testing.T) {
	output := filepath.Join(t.TempDir(), "reader.json")
	counter := filepath.Join(t.TempDir(), "attempts")
	t.Setenv("PANDORA_READER_HELPER", "1")
	t.Setenv("PANDORA_READER_HELPER_OUTPUT", output)
	t.Setenv("PANDORA_READER_HELPER_COUNTER", counter)
	t.Setenv("PANDORA_READER_HELPER_FAILURES", "0")
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	f.addReader(t, config.EventReader{Name: "deliver", Command: testReaderCommand(), TimeoutSec: 10})
	state, err := f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if state.Readers != 1 {
		t.Fatalf("ReloadWebhooks() = %+v, want one reader", state)
	}
	event := f.appendEvent(t, "one", "local", "topic")

	result := waitForTestReaderOutput(t, output)
	if result.Reader != "deliver" {
		t.Fatalf("PANDORA_READER = %q, want deliver", result.Reader)
	}
	if result.Seq != strconv.FormatInt(event.Seq, 10) || result.ID != event.ID {
		t.Fatalf("event env = seq %q id %q", result.Seq, result.ID)
	}
	if result.Service != "one" || result.Account != "local" || result.Topic != "topic" {
		t.Fatalf("event env = %s/%s/%s", result.Service, result.Account, result.Topic)
	}
	var stdinEvent events.Event
	if err := json.Unmarshal([]byte(result.Stdin), &stdinEvent); err != nil {
		t.Fatalf("decoding stdin event: %v", err)
	}
	if stdinEvent.Seq != event.Seq || stdinEvent.Service != event.Service || stdinEvent.Topic != event.Topic {
		t.Fatalf("stdin event = %+v", stdinEvent)
	}
	waitForWebhookCondition(t, 2*time.Second, func() bool {
		pending, err := f.core.PendingEvents(context.Background(), "deliver", events.Filter{})
		return err == nil && len(pending) == 0
	}, "the reader did not acknowledge the event")
}

func TestAReaderRetriesAfterAFailureAndThenAcknowledges(t *testing.T) {
	output := filepath.Join(t.TempDir(), "reader.json")
	counter := filepath.Join(t.TempDir(), "attempts")
	t.Setenv("PANDORA_READER_HELPER", "1")
	t.Setenv("PANDORA_READER_HELPER_OUTPUT", output)
	t.Setenv("PANDORA_READER_HELPER_COUNTER", counter)
	t.Setenv("PANDORA_READER_HELPER_FAILURES", "2")
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	f.addReader(t, config.EventReader{Name: "deliver", Command: testReaderCommand(), TimeoutSec: 10})
	if _, err := f.client.ReloadWebhooks(context.Background()); err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	event := f.appendEvent(t, "one", "local", "topic")

	_ = waitForTestReaderOutput(t, output)
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("reading attempts: %v", err)
	}
	attempts, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parsing attempts: %v", err)
	}
	if attempts < 3 {
		t.Fatalf("reader attempts = %d, want at least 3", attempts)
	}
	waitForWebhookCondition(t, 2*time.Second, func() bool {
		pending, err := f.core.PendingEvents(context.Background(), "deliver", events.Filter{})
		return err == nil && len(pending) == 0
	}, "the reader did not acknowledge after retrying")
	if event.Seq == 0 {
		t.Fatal("the event has no sequence")
	}
}

func TestARemovedReaderStopsReceivingEvents(t *testing.T) {
	output := filepath.Join(t.TempDir(), "reader.json")
	counter := filepath.Join(t.TempDir(), "attempts")
	t.Setenv("PANDORA_READER_HELPER", "1")
	t.Setenv("PANDORA_READER_HELPER_OUTPUT", output)
	t.Setenv("PANDORA_READER_HELPER_COUNTER", counter)
	t.Setenv("PANDORA_READER_HELPER_FAILURES", "0")
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	f.addReader(t, config.EventReader{Name: "deliver", Command: testReaderCommand(), TimeoutSec: 10})
	if _, err := f.client.ReloadWebhooks(context.Background()); err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	f.appendEvent(t, "one", "local", "first")
	_ = waitForTestReaderOutput(t, output)
	waitForWebhookCondition(t, 2*time.Second, func() bool {
		pending, err := f.core.PendingEvents(context.Background(), "deliver", events.Filter{})
		return err == nil && len(pending) == 0
	}, "the reader did not acknowledge the first event")

	f.removeReader(t, "deliver")
	state, err := f.client.ReloadWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ReloadWebhooks() error = %v", err)
	}
	if state.Readers != 0 {
		t.Fatalf("ReloadWebhooks() = %+v, want no readers", state)
	}
	if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
		t.Fatalf("removing reader output: %v", err)
	}
	f.appendEvent(t, "one", "local", "second")
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("removed reader still wrote output: %v", err)
	}
}

func TestASecondDaemonReturnsErrAlreadyRunning(t *testing.T) {
	f := newWebhookDaemonFixture(t, 5*time.Minute)
	second := NewServer(f.core, "test", 5*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := second.Serve(ctx, f.paths.DaemonSocket()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second daemon error = %v, want ErrAlreadyRunning", err)
	}
	if _, err := f.client.Health(ctx); err != nil {
		t.Fatalf("the running daemon stopped answering: %v", err)
	}
}
