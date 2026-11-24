package tunnel_test

import (
	"context"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

func TestStaticRefusesAnHTTPBase(t *testing.T) {
	static := tunnel.Static{Base: "http://hooks.example.com"}

	_, err := static.Expose(context.Background(), 8080, "/pandora")
	if err == nil {
		t.Fatal("Expose() accepted an http:// base")
	}
	want := "public URL must start with https://"
	if err.Error() != want {
		t.Fatalf("Expose() error = %q, want %q", err, want)
	}
}

func TestStaticReturnsTheConfiguredHTTPSBase(t *testing.T) {
	static := tunnel.Static{Base: "https://hooks.example.com/"}

	base, err := static.Expose(context.Background(), 8080, "/pandora")
	if err != nil {
		t.Fatalf("Expose() error = %v", err)
	}
	if base != "https://hooks.example.com" {
		t.Fatalf("Expose() base = %q", base)
	}

	status, err := static.Check(context.Background(), 8080, "/pandora")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.Exposed || status.PublicBase != base || status.Problem != "" {
		t.Fatalf("Check() = %+v", status)
	}
	if static.Name() != "url" {
		t.Fatalf("Name() = %q, want url", static.Name())
	}
	if err := static.Unexpose(context.Background(), "/pandora"); err != nil {
		t.Fatalf("Unexpose() error = %v", err)
	}
}
