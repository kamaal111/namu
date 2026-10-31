package testutil

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

// GitHub installs a gh double and records its arguments without using credentials.
func GitHub(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	t.Setenv("GH_CALLS", log)
	path := filepath.Join(bin, "gh")
	Write(t, path, githubScript+body)
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

//go:embed scripts/github.sh
var githubScript string
