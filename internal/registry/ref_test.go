package registry_test

import (
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func TestNamesAcceptOnlyLowercaseLettersDigitsAndDashes(t *testing.T) {
	valid := []string{
		"a",
		"todoist",
		"my-service-1",
		strings.Repeat("a", 32),
	}
	for _, name := range valid {
		if err := registry.ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{
		"",
		"-a",
		"A",
		"a_b",
		"a b",
		"a/b",
		"a.b",
		strings.Repeat("a", 33),
	}
	for _, name := range invalid {
		if err := registry.ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", name)
		}
	}
}

func TestParseRefReadsServiceAndAccountNames(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    registry.Ref
		wantErr bool
	}{
		{name: "service only", input: "todoist", want: registry.Ref{Service: "todoist"}},
		{name: "service and account", input: "todoist/personal", want: registry.Ref{Service: "todoist", Account: "personal"}},
		{name: "empty", input: "", wantErr: true},
		{name: "empty service", input: "/personal", wantErr: true},
		{name: "empty account", input: "todoist/", wantErr: true},
		{name: "too many parts", input: "a/b/c", wantErr: true},
		{name: "uppercase service", input: "Todoist", wantErr: true},
		{name: "uppercase account", input: "todoist/Pessoal", wantErr: true},
		{name: "space in name", input: "todo ist", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registry.ParseRef(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRef(%q) = %v, nil; want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRef(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseRef(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
			if got.String() != tt.input {
				t.Fatalf("Ref.String() = %q, want %q", got.String(), tt.input)
			}
		})
	}
}
