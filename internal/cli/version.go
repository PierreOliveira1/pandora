package cli

import (
	"runtime/debug"
	"strings"
)

var version = "dev"

func resolveVersion(linked string, info *debug.BuildInfo, ok bool) string {
	if linked != "dev" {
		return linked
	}
	if !ok || info == nil {
		return "dev"
	}
	moduleVersion := info.Main.Version
	if moduleVersion == "" || moduleVersion == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(moduleVersion, "v")
}

func appVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}
