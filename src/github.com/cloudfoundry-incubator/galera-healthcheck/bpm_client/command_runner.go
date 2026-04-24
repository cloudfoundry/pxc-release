package bpm_client

import (
	"context"
	"os/exec"
	"time"
)

type RealCommandRunner struct {
	Timeout time.Duration
}

func NewRealCommandRunner(timeout time.Duration) *RealCommandRunner {
	return &RealCommandRunner{
		Timeout: timeout,
	}
}

func (r *RealCommandRunner) Run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}