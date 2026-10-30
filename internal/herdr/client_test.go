package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func executable(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestRunReturnsCommandDiagnostics(t *testing.T) {
	executable(t, "echo rejected >&2; exit 7")
	err := Run(context.Background(), "worktree", "create")
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected failure diagnostics, got %v", err)
	}
}

func TestRunMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Run(context.Background(), "worktree", "create")
	if err == nil {
		t.Fatal("missing executable must fail")
	}
}

func TestRunCancellation(t *testing.T) {
	executable(t, "exec /bin/sleep 60")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, "worktree", "create")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestRunTimeout(t *testing.T) {
	executable(t, "exec /bin/sleep 60")
	err := run(context.Background(), 10*time.Millisecond, "worktree", "create")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
}
