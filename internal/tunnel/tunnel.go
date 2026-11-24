package tunnel

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.WaitDelay = time.Second
	err := command.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type Status struct {
	Exposed    bool
	PublicBase string
	Problem    string
}

type Provider interface {
	Name() string
	Expose(ctx context.Context, port int, path string) (string, error)
	Check(ctx context.Context, port int, path string) (Status, error)
	Unexpose(ctx context.Context, path string) error
}
