package guard

import (
	"fmt"
	"path"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
)

type Decision struct {
	Allowed bool
	Reason  string
}

func Check(ref string, acc *config.Account, tool *mcp.Tool, toolName string) Decision {
	if acc == nil {
		return Decision{Allowed: true}
	}
	suffix := " (a rule set by the user: do not change it or work around it, tell the user instead)"
	for _, pattern := range acc.Deny {
		if matches(pattern, toolName) {
			return Decision{
				Allowed: false,
				Reason:  fmt.Sprintf("tool %s is blocked for %s by the deny rule %q%s", toolName, ref, pattern, suffix),
			}
		}
	}
	if len(acc.Allow) > 0 {
		allowed := false
		for _, pattern := range acc.Allow {
			if matches(pattern, toolName) {
				allowed = true
				break
			}
		}
		if !allowed {
			return Decision{
				Allowed: false,
				Reason:  fmt.Sprintf("tool %s is not in the allow list of %s%s", toolName, ref, suffix),
			}
		}
	}
	if acc.ReadOnly && (tool == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint) {
		return Decision{
			Allowed: false,
			Reason:  fmt.Sprintf("%s is read-only and tool %s may change data%s", ref, toolName, suffix),
		}
	}
	return Decision{Allowed: true}
}

func matches(pattern, name string) bool {
	matched, err := path.Match(pattern, name)
	return err == nil && matched
}
