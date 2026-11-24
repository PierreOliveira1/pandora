//go:build darwin

package tunnel_test

import (
	"slices"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

func TestNewTailscaleUsesTheMacAppCandidate(t *testing.T) {
	ts := tunnel.NewTailscale(&fakeRunner{})
	want := []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale"}
	if !slices.Equal(ts.Candidates, want) {
		t.Fatalf("Candidates = %v, want %v", ts.Candidates, want)
	}
}
