package registry_test

import (
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func TestACommandToolCanOnlyUseParametersItDeclares(t *testing.T) {
	f := newFixture(t)
	if err := f.reg.Add(registry.AddInput{
		Service: "git",
		Account: "local",
		Kind:    config.KindCommand,
	}); err != nil {
		t.Fatalf("adding command service: %v", err)
	}

	err := f.reg.AddCommandTool(registry.CommandToolInput{
		Service: "git",
		Name:    "log",
		Command: []string{"git", "-C", "{{repo}}", "log", "{{missing}}"},
		Params: map[string]config.CommandParam{
			"repo": {Type: "string", Required: true},
		},
	})
	if err == nil {
		t.Fatal("AddCommandTool() succeeded, want an unknown parameter error")
	}
	if !strings.Contains(err.Error(), "unknown parameter missing") {
		t.Errorf("AddCommandTool() error = %v, want unknown parameter missing", err)
	}
}
