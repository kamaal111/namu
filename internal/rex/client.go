// Package rex owns the optional Rex CLI boundary and worktree session identity.
package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "rex", args...)
	cmd.WaitDelay = time.Second
	var output, diagnostics bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostics
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("rex %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(diagnostics.String()+output.String()), err)
	}
	return output.Bytes(), nil
}

func readJSON(ctx context.Context, target any, args ...string) error {
	output, err := run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(output, target); err != nil {
		return fmt.Errorf("invalid rex %s response: %w", strings.Join(args, " "), err)
	}
	return nil
}
