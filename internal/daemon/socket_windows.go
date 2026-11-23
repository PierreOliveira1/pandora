//go:build windows

package daemon

func restrictSocket(string) error {
	return nil
}
