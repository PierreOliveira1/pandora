//go:build !windows

package daemon

import "os"

func restrictSocket(path string) error {
	return os.Chmod(path, 0o600)
}
