package cli

import (
	"runtime/debug"
	"testing"
)

func TestAReleaseBuildReportsTheVersionItWasTaggedWith(t *testing.T) {
	tests := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		ok     bool
		want   string
	}{
		{
			name:   "release linked with ldflags",
			linked: "1.2.3",
			want:   "1.2.3",
		},
		{
			name:   "installed with go install at a version",
			linked: "dev",
			info:   &debug.BuildInfo{Main: debug.Module{Version: "v1.4.0"}},
			ok:     true,
			want:   "1.4.0",
		},
		{
			name:   "built from a development tree",
			linked: "dev",
			info:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			ok:     true,
			want:   "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.linked, tt.info, tt.ok); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
