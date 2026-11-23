package secrets_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/secrets"
)

func TestFileStoreKeepsValuesAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	first := secrets.NewFileStore(path)
	if err := first.Set("gh/personal/env/TOKEN", "abc"); err != nil {
		t.Fatalf("setting secret: %v", err)
	}

	second := secrets.NewFileStore(path)
	value, err := second.Get("gh/personal/env/TOKEN")
	if err != nil {
		t.Fatalf("getting secret from a new instance: %v", err)
	}
	if value != "abc" {
		t.Fatalf("got %q, want %q", value, "abc")
	}

	if err := second.Delete("gh/personal/env/TOKEN"); err != nil {
		t.Fatalf("deleting secret: %v", err)
	}
	if _, err := first.Get("gh/personal/env/TOKEN"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("after delete got error %v, want %v", err, secrets.ErrNotFound)
	}
}
