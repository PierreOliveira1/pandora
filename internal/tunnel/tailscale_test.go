package tunnel_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

func TestTailscaleWithoutCLIExplainsHowToInstall(t *testing.T) {
	runner := &fakeRunner{}
	ts := tunnel.Tailscale{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Exists:   func(string) bool { return false },
	}

	_, err := ts.Expose(context.Background(), 8080, "/pandora")
	if err == nil {
		t.Fatal("Expose() succeeded without a Tailscale CLI")
	}
	want := "Tailscale is not installed: install it from https://tailscale.com/download and sign in"
	if err.Error() != want {
		t.Fatalf("Expose() error = %q, want %q", err, want)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %v, want none", runner.calls)
	}
}

func TestTailscaleUsesACandidateWhenTheCLIIsNotOnPath(t *testing.T) {
	candidate := filepath.Join(t.TempDir(), "Tailscale")
	if err := os.WriteFile(candidate, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing candidate: %v", err)
	}
	runner := &fakeRunner{results: []runnerResult{
		{stdout: stateJSON("Running", "host.tailnet.ts.net.")},
		{stdout: []byte("available")},
	}}
	ts := tunnel.Tailscale{
		Runner:     runner,
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		Candidates: []string{candidate},
	}

	base, err := ts.Expose(context.Background(), 8080, "/pandora")
	if err != nil {
		t.Fatalf("Expose() error = %v", err)
	}
	if base != "https://host.tailnet.ts.net/pandora" {
		t.Fatalf("Expose() base = %q", base)
	}
	if len(runner.calls) != 2 || runner.calls[0].name != candidate || runner.calls[1].name != candidate {
		t.Fatalf("runner calls = %v, want both on %s", runner.calls, candidate)
	}
}

func TestTailscaleDisconnectedExplainsHowToSignIn(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{
		{stdout: stateJSON("Stopped", "host.tailnet.ts.net.")},
	}}
	ts := tunnel.Tailscale{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/tailscale", nil },
	}

	_, err := ts.Expose(context.Background(), 8080, "/pandora")
	if err == nil {
		t.Fatal("Expose() succeeded while Tailscale was stopped")
	}
	want := "Tailscale is not connected: open Tailscale and sign in"
	if err.Error() != want {
		t.Fatalf("Expose() error = %q, want %q", err, want)
	}
}

func TestExposePublishesTheLocalPortAtTheFunnelPath(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{
		{stdout: stateJSON("Running", "host.tailnet.ts.net.")},
		{stdout: []byte("Available on the internet:\nhttps://host.tailnet.ts.net/pandora\n")},
	}}
	ts := tunnel.Tailscale{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/local/bin/tailscale", nil },
	}

	base, err := ts.Expose(context.Background(), 8080, "/pandora")
	if err != nil {
		t.Fatalf("Expose() error = %v", err)
	}
	if base != "https://host.tailnet.ts.net/pandora" {
		t.Fatalf("Expose() base = %q", base)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %v, want 2", runner.calls)
	}
	if got := runner.calls[0]; got.name != "/usr/local/bin/tailscale" || !slices.Equal(got.args, []string{"status", "--json"}) {
		t.Fatalf("state call = %v", got)
	}
	want := []string{
		"funnel", "--bg", "--yes", "--https=443", "--set-path=/pandora",
		"http://127.0.0.1:8080",
	}
	if got := runner.calls[1]; got.name != "/usr/local/bin/tailscale" || !slices.Equal(got.args, want) {
		t.Fatalf("funnel call = %v, want %v", got, want)
	}
}

func TestExposeShowsTheFunnelEnableLinkWhenTailscaleAsksForIt(t *testing.T) {
	link := "https://login.tailscale.com/f/funnel?node=abc"
	runner := &fakeRunner{results: []runnerResult{
		{stdout: stateJSON("Running", "host.tailnet.ts.net.")},
		{
			stderr: []byte("Enable Funnel here: " + link + "\n"),
			err:    context.DeadlineExceeded,
		},
	}}
	ts := tunnel.Tailscale{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/tailscale", nil },
	}

	_, err := ts.Expose(context.Background(), 8080, "/pandora")
	if err == nil {
		t.Fatal("Expose() succeeded without Funnel enabled")
	}
	want := "Tailscale Funnel is not enabled for this machine: open " + link + ", enable it, then run the command again"
	if err.Error() != want {
		t.Fatalf("Expose() error = %q, want %q", err, want)
	}
}

func TestCheckRecognizesTheFunnelMapping(t *testing.T) {
	const (
		host     = "host.tailnet.ts.net:443"
		path     = "/pandora"
		proxy    = "http://127.0.0.1:8080"
		wantBase = "https://host.tailnet.ts.net/pandora"
	)
	tests := []struct {
		name        string
		status      []byte
		wantExposed bool
	}{
		{
			name:        "matching proxy and allow funnel",
			status:      funnelJSON(host, path, proxy, true),
			wantExposed: true,
		},
		{
			name:   "another port",
			status: funnelJSON(host, path, "http://127.0.0.1:9999", true),
		},
		{
			name:   "allow funnel false",
			status: funnelJSON(host, path, proxy, false),
		},
		{
			name:   "another path",
			status: funnelJSON(host, "/other", proxy, true),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{results: []runnerResult{
				{stdout: stateJSON("Running", "host.tailnet.ts.net.")},
				{stdout: test.status},
			}}
			ts := tunnel.Tailscale{
				Runner:   runner,
				LookPath: func(string) (string, error) { return "/usr/bin/tailscale", nil },
			}
			status, err := ts.Check(context.Background(), 8080, path)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if status.Exposed != test.wantExposed {
				t.Fatalf("Exposed = %v, want %v", status.Exposed, test.wantExposed)
			}
			if status.PublicBase != wantBase {
				t.Fatalf("PublicBase = %q, want %q", status.PublicBase, wantBase)
			}
			if test.wantExposed {
				if status.Problem != "" {
					t.Fatalf("Problem = %q, want empty", status.Problem)
				}
				return
			}
			wantProblem := "Tailscale Funnel is not serving /pandora: run pandora tunnel use tailscale"
			if status.Problem != wantProblem {
				t.Fatalf("Problem = %q, want %q", status.Problem, wantProblem)
			}
		})
	}
}

func TestCheckRejectsAMalformedFunnelStatus(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{
		{stdout: stateJSON("Running", "host.tailnet.ts.net.")},
		{stdout: []byte("not json")},
	}}
	ts := tunnel.Tailscale{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/tailscale", nil },
	}

	_, err := ts.Check(context.Background(), 8080, "/pandora")
	if err == nil || !strings.Contains(err.Error(), "reading Tailscale Funnel status") {
		t.Fatalf("Check() error = %v", err)
	}
}
