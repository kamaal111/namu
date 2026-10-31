package testutil

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

// Rex installs a CLI double that records each invocation as separated arguments.
func Rex(t testing.TB, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	t.Setenv("REX_CALLS", log)
	t.Setenv("REX_CLIENT", "")
	t.Setenv("REX_SESSION", "session:source")
	t.Setenv("REX_BLOCK", "block:source")
	t.Setenv("HERDR_ENV", "")
	Write(t, filepath.Join(bin, "rex"), rexScript+body)
	if err := os.Chmod(filepath.Join(bin, "rex"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// RexResponses provides a live session list after the first successful creation.
//
//go:embed scripts/rex-responses.sh
var RexResponses string

//go:embed scripts/rex.sh
var rexScript string
