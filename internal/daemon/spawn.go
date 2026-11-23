package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func Ensure(ctx context.Context, paths config.Paths, exe, version string) (*Client, error) {
	socketPath := paths.DaemonSocket()
	if client, err := Dial(ctx, socketPath); err == nil {
		health, healthErr := client.Health(ctx)
		if healthErr == nil && health.Version == version {
			return client, nil
		}
		if healthErr == nil {
			_ = client.Shutdown(ctx)
			waitForStop(ctx, socketPath)
		}
	}

	if err := startDaemon(paths, exe); err != nil {
		return nil, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for {
		if client, err := Dial(waitCtx, socketPath); err == nil {
			health, healthErr := client.Health(waitCtx)
			if healthErr == nil && health.Version == version {
				return client, nil
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("daemon did not start: %w", waitCtx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func startDaemon(paths config.Paths, exe string) error {
	if exe == "" {
		return errors.New("pandora executable path is empty")
	}
	logPath := paths.DaemonLog()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("creating daemon log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening daemon log: %w", err)
	}
	defer logFile.Close()

	command := exec.Command(exe, "daemon", "--idle", "15m")
	command.Stdin = nil
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = detachedProcess()
	if err := command.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("detaching daemon: %w", err)
	}
	return nil
}

func waitForStop(ctx context.Context, socketPath string) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := Dial(ctx, socketPath); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}
