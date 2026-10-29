// Package testutil provides isolated local Git fixtures for integration tests.
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/kamaal111/namu/internal/git"
)

func Write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
	return strings.TrimSpace(string(output))
}
func Repository(t testing.TB) git.Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	source := t.TempDir()
	Git(t, source, "init", "-qb", "main")
	Write(t, filepath.Join(source, "tracked.txt"), "initial")
	Git(t, source, "add", ".")
	Git(t, source, "commit", "-qm", "initial")
	repo, err := git.Discover(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}
