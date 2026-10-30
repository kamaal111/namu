package testutil

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Herdr installs an isolated CLI double; body can perform real Git operations.
func Herdr(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(bin, "calls")
	t.Setenv("REAL_GIT", realGit)
	t.Setenv("HERDR_CALLS", log)
	Write(t, filepath.Join(bin, "herdr"), herdrScript+body)
	if err := os.Chmod(filepath.Join(bin, "herdr"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

//go:embed scripts/herdr.sh
var herdrScript string
