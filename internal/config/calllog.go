package config

import "path/filepath"

func (p Paths) CallLog() string {
	return filepath.Join(p.StateDir, "calls.jsonl")
}
