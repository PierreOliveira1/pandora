package core_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func TestAReadOnlyAccountRefusesToolsThatMayChangeDataAndEveryCallIsLogged(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "ana")
	readOnly := true
	if _, err := f.reg.SetPolicy(registry.Ref{Service: "fake", Account: "a"}, registry.PolicyInput{ReadOnly: &readOnly}); err != nil {
		t.Fatalf("setting policy: %v", err)
	}
	callLog := guard.NewLog(filepath.Join(t.TempDir(), "calls.jsonl"))
	app := core.New(core.Options{Store: f.store, Secrets: f.secrets, Openers: f.openers, Version: "test", CallLog: callLog})
	t.Cleanup(func() { _ = app.Close() })
	ctx := core.WithClient(context.Background(), "claude-code")

	if _, err := app.Call(ctx, "fake", "echo", map[string]any{"text": "hi"}); err != nil {
		t.Fatalf("calling a read-only tool: %v", err)
	}
	_, err := app.Call(ctx, "fake", "add", map[string]any{"a": 1, "b": 2})
	var blocked *core.BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("calling a tool that may change data: err = %v, want a blocked error", err)
	}

	entries, err := callLog.Tail(10, guard.Filter{})
	if err != nil {
		t.Fatalf("reading the call log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d log entries, want 2", len(entries))
	}
	if !entries[0].OK || entries[0].Ref != "fake/a" || entries[0].Tool != "echo" || entries[0].Client != "claude-code" {
		t.Fatalf("first entry = %+v, want an ok echo call on fake/a from claude-code", entries[0])
	}
	if entries[1].OK || !entries[1].Blocked || entries[1].Tool != "add" {
		t.Fatalf("second entry = %+v, want a blocked add call", entries[1])
	}
}
