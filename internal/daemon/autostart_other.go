//go:build !darwin && !linux

package daemon

import (
	"context"
	"fmt"
	"runtime"
)

func (a Autostart) Enable(ctx context.Context) error {
	return fmt.Errorf("starting Pandora at login is not supported on %s yet", runtime.GOOS)
}

func (a Autostart) Disable(ctx context.Context) error {
	return fmt.Errorf("starting Pandora at login is not supported on %s yet", runtime.GOOS)
}

func (a Autostart) Enabled(ctx context.Context) (bool, error) {
	return false, nil
}
