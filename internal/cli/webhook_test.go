package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PierreOliveira1/pandora/internal/daemon"
	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/secrets"
)

func newCLIHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PANDORA_HOME", home)
	t.Setenv("PANDORA_SECRETS", "file")
	t.Setenv("PANDORA_NO_DAEMON", "1")
	t.Setenv("HOME", filepath.Join(home, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	return home
}

func executePandora(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func runPandora(t *testing.T, stdin string, args ...string) (string, string) {
	t.Helper()
	code, stdout, stderr := executePandora(t, stdin, args...)
	if code != 0 {
		t.Fatalf("%v exited with %d: stdout=%s stderr=%s", args, code, stdout, stderr)
	}
	return stdout, stderr
}

func addCommandAccount(t *testing.T, service, account string) {
	t.Helper()
	runPandora(t, "", "add", service, "--as", account, "--kind", "command")
}

func appendTestEvent(t *testing.T, home, service, account, topic, payload string) events.Event {
	t.Helper()
	queue := events.NewQueue(filepath.Join(home, "state", "events"), time.Now)
	event, duplicate, err := queue.Append(context.Background(), events.Event{
		Service: service,
		Account: account,
		Topic:   topic,
		Payload: json.RawMessage(payload),
	}, "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if duplicate {
		t.Fatal("Append() duplicate = true, want false")
	}
	return event
}

func secretFromFile(t *testing.T, home, key string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "config", "secrets.json"))
	if err != nil {
		t.Fatalf("reading secrets file: %v", err)
	}
	values := map[string]string{}
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatalf("parsing secrets file: %v", err)
	}
	return values[key]
}

func TestAddingAnHMACWebhookPrintsTheCallbackURLAndNeverTheSecret(t *testing.T) {
	newCLIHome(t)
	addCommandAccount(t, "ig", "store")
	runPandora(t, "", "tunnel", "use", "url", "https://hooks.example.com")

	stdout, _ := runPandora(t, "super-secret\n", "webhook", "add", "ig/store", "--style", "hmac", "--challenge", "hub")
	for _, want := range []string{
		"Webhook added to ig/store.",
		"Callback URL: https://hooks.example.com/hooks/ig/store/",
		"Verify token: ",
		"Paste them in the service's webhook settings, then check with: pandora webhook test ig/store",
		"Events: pandora events ig/store",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("webhook add output does not contain %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "super-secret") {
		t.Errorf("webhook add output contains the signing secret:\n%s", stdout)
	}
}

func TestATokenWebhookDoesNotPrintItsTokenOutsideATerminal(t *testing.T) {
	home := newCLIHome(t)
	addCommandAccount(t, "ig", "store")

	stdout, _ := runPandora(t, "", "webhook", "add", "ig/store", "--style", "token", "--token-header", "X-Token")
	want := "Token: run pandora webhook show ig/store --token in your terminal to see it"
	if !strings.Contains(stdout, want) {
		t.Errorf("webhook add output does not contain %q:\n%s", want, stdout)
	}
	token := secretFromFile(t, home, secrets.WebhookSigningKey("ig", "store"))
	if token == "" {
		t.Fatal("token webhook stored no signing secret")
	}
	if strings.Contains(stdout, token) {
		t.Errorf("webhook add output contains the token:\n%s", stdout)
	}

	code, showOut, stderr := executePandora(t, "", "webhook", "show", "ig/store", "--token")
	if code == 0 {
		t.Fatal("webhook show --token succeeded outside a terminal")
	}
	if !strings.Contains(stderr, "the token is only shown in a terminal: run this command yourself") {
		t.Errorf("webhook show stderr = %q, want the terminal restriction", stderr)
	}
	if strings.Contains(showOut, token) || strings.Contains(stderr, token) {
		t.Error("webhook show printed the token outside a terminal")
	}
}

func TestWebhookListShowsTheWebhook(t *testing.T) {
	newCLIHome(t)
	addCommandAccount(t, "ig", "store")
	runPandora(t, "secret\n", "webhook", "add", "ig/store", "--style", "hmac")

	stdout, _ := runPandora(t, "", "webhook", "list")
	if !strings.Contains(stdout, "ig/store") || !strings.Contains(stdout, "hmac") {
		t.Fatalf("webhook list output = %q, want the webhook", stdout)
	}
}

func TestEventsShowPendingEventsUntilTheyAreAcknowledged(t *testing.T) {
	home := newCLIHome(t)
	appendTestEvent(t, home, "ig", "store", "comments", `{"hello":"world"}`)

	stdout, _ := runPandora(t, "", "events")
	for _, want := range []string{
		"#1",
		"ig/store",
		"comments",
		`{"hello":"world"}`,
		"Mark them as read: pandora events ack 1 --reader default",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("events output does not contain %q:\n%s", want, stdout)
		}
	}

	runPandora(t, "", "events", "ack", "1")
	stdout, _ = runPandora(t, "", "events")
	if !strings.Contains(stdout, "No new events.") || strings.Contains(stdout, "#1") {
		t.Fatalf("events after ack = %q, want no pending events", stdout)
	}
}

func TestEventsJSONOutputIsAValidArray(t *testing.T) {
	home := newCLIHome(t)
	appendTestEvent(t, home, "ig", "store", "comments", `{"hello":"world"}`)

	stdout, _ := runPandora(t, "", "events", "--json")
	var list []events.Event
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("events --json output is not valid JSON: %v\n%s", err, stdout)
	}
	if len(list) != 1 || list[0].Seq != 1 || list[0].Service != "ig" {
		t.Fatalf("events --json list = %+v, want the pending event", list)
	}
}

func TestEventsAckFlagMarksThePrintedEvents(t *testing.T) {
	home := newCLIHome(t)
	appendTestEvent(t, home, "ig", "store", "comments", `{"n":1}`)
	appendTestEvent(t, home, "ig", "store", "comments", `{"n":2}`)

	stdout, _ := runPandora(t, "", "events", "--ack")
	if !strings.Contains(stdout, "#1") || !strings.Contains(stdout, "#2") {
		t.Fatalf("events --ack output = %q, want both events", stdout)
	}
	if strings.Contains(stdout, "Mark them as read") {
		t.Errorf("events --ack printed the ack hint:\n%s", stdout)
	}
	stdout, _ = runPandora(t, "", "events")
	if !strings.Contains(stdout, "No new events.") {
		t.Fatalf("events after --ack = %q, want no pending events", stdout)
	}
}

func TestAReaderNameCannotBeReserved(t *testing.T) {
	newCLIHome(t)
	code, _, stderr := executePandora(t, "", "events", "reader", "add", "ai", "--", "echo")
	if code == 0 {
		t.Fatal("reserved reader name was accepted")
	}
	if !strings.Contains(stderr, "reader name ai is reserved: choose another name") {
		t.Fatalf("stderr = %q, want the reserved reader error", stderr)
	}
}

func TestAReaderIsAddedAndListed(t *testing.T) {
	newCLIHome(t)
	addCommandAccount(t, "ig", "store")

	stdout, _ := runPandora(t, "", "events", "reader", "add", "worker", "--from", "ig/store", "--topic", "comments", "--", "echo", "hello")
	if !strings.Contains(stdout, "Reader worker added. The daemon runs it for each new event, with the event as JSON on stdin.") {
		t.Fatalf("reader add output = %q", stdout)
	}
	stdout, _ = runPandora(t, "", "events", "reader", "list")
	for _, want := range []string{"worker", "ig/store", "comments", "echo hello"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("reader list output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestRemovingAWebhookTellsTheUserToRemoveTheCallbackURL(t *testing.T) {
	newCLIHome(t)
	addCommandAccount(t, "ig", "store")
	runPandora(t, "secret\n", "webhook", "add", "ig/store", "--style", "hmac")

	stdout, _ := runPandora(t, "", "webhook", "remove", "ig/store")
	for _, want := range []string{
		"Webhook removed from ig/store.",
		"Remove the callback URL from the service's settings too.",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("webhook remove output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestTunnelUseURLRefusesHTTP(t *testing.T) {
	newCLIHome(t)
	code, _, stderr := executePandora(t, "", "tunnel", "use", "url", "http://hooks.example.com")
	if code == 0 {
		t.Fatal("tunnel use url accepted an http:// base")
	}
	if !strings.Contains(stderr, "https://") {
		t.Fatalf("stderr = %q, want the https requirement", stderr)
	}
}

func TestRenamingAnAccountWarnsThatTheWebhookURLChanged(t *testing.T) {
	newCLIHome(t)
	addCommandAccount(t, "ig", "store")
	runPandora(t, "", "tunnel", "use", "url", "https://hooks.example.com")
	runPandora(t, "secret\n", "webhook", "add", "ig/store", "--style", "hmac")

	stdout, stderr := runPandora(t, "", "rename", "ig/store", "loja")
	if !strings.Contains(stderr, "warning: the webhook URL of ig/loja changed to https://hooks.example.com/hooks/ig/loja/") {
		t.Fatalf("rename stderr does not contain the changed webhook URL:\n%s", stderr)
	}
	if !strings.Contains(stderr, "update it in the service") {
		t.Fatalf("rename stderr does not tell the user to update the service:\n%s", stderr)
	}
	if strings.Contains(stdout, "warning: the webhook URL") {
		t.Fatalf("rename stdout contains the warning:\n%s", stdout)
	}
}

func TestAddingAWebhookToAnAccountThatAlreadyHasOneFailsBeforeAskingForTheSecret(t *testing.T) {
	newCLIHome(t)
	addCommandAccount(t, "ig", "store")
	runPandora(t, "secret\n", "webhook", "add", "ig/store", "--style", "hmac")

	code, _, stderr := executePandora(t, "", "webhook", "add", "ig/store", "--style", "hmac")
	if code == 0 {
		t.Fatal("second webhook add succeeded")
	}
	if !strings.Contains(stderr, "already has a webhook") {
		t.Fatalf("stderr = %q, want the already has a webhook error", stderr)
	}
	if strings.Contains(stderr, "value for WEBHOOK_SECRET required") {
		t.Fatalf("stderr asked for the secret before checking the webhook:\n%s", stderr)
	}
}

type autostartRunner struct {
	calls [][]string
}

func (r *autostartRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil, nil, nil
}

func TestAutostartOnUsesTheConfiguredConstructor(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("autostart is only supported on macOS and Linux")
	}
	home := t.TempDir()
	runner := &autostartRunner{}
	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		autostart: func() (daemon.Autostart, error) {
			return daemon.Autostart{
				Exe:        "/opt/pandora/bin/pandora",
				Home:       home,
				Env:        map[string]string{"PATH": "/usr/bin", "HOME": home},
				LogPath:    filepath.Join(home, "daemon.log"),
				UID:        501,
				ConfigHome: filepath.Join(home, ".config"),
				Runner:     runner,
			}, nil
		},
	}
	cmd := newAutostart(d)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"on"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("autostart on error = %v", err)
	}
	if len(runner.calls) == 0 {
		t.Fatal("autostart on did not call the runner")
	}
	if !strings.Contains(stdout.String(), "Pandora now starts at login.") {
		t.Fatalf("autostart on output = %q", stdout.String())
	}
}

func TestANewReaderSkipsTheEventsAlreadyInTheQueueUnlessAskedToReplay(t *testing.T) {
	home := newCLIHome(t)
	addCommandAccount(t, "ig", "store")
	old := appendTestEvent(t, home, "ig", "store", "comments", `{"text":"old"}`)

	runPandora(t, "", "events", "reader", "add", "replier", "--", "echo", "hello")
	stdout, _ := runPandora(t, "", "events", "--reader", "replier")
	if !strings.Contains(stdout, "No new events.") {
		t.Fatalf("a new reader sees old events:\n%s", stdout)
	}
	fresh := appendTestEvent(t, home, "ig", "store", "comments", `{"text":"new"}`)
	stdout, _ = runPandora(t, "", "events", "--reader", "replier")
	if !strings.Contains(stdout, fmt.Sprintf("#%d ", fresh.Seq)) || strings.Contains(stdout, fmt.Sprintf("#%d ", old.Seq)) {
		t.Fatalf("reader output should list only the new event:\n%s", stdout)
	}

	runPandora(t, "", "events", "reader", "add", "archiver", "--replay", "--", "echo", "hello")
	stdout, _ = runPandora(t, "", "events", "--reader", "archiver")
	if !strings.Contains(stdout, fmt.Sprintf("#%d ", old.Seq)) {
		t.Fatalf("a replaying reader does not see the old event:\n%s", stdout)
	}
}
