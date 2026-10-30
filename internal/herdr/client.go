// Package herdr owns the optional Herdr CLI boundary.
package herdr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Run leaves session/socket selection to Herdr's inherited environment.
func Run(ctx context.Context, args ...string) error {
	return run(ctx, 60*time.Second, args...)
}

func run(ctx context.Context, timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", args...)
	cmd.WaitDelay = time.Second
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("herdr %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(output.String()), err)
	}
	return nil
}
