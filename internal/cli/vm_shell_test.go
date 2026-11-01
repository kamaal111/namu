package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVMShellWrapperKeepsTerminalStreamsAndParentDirectory(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			dir := t.TempDir()
			executable := filepath.Join(dir, "namu")
			must(t, os.WriteFile(executable, []byte("#!/bin/sh\n[ \"$1\" = open ] && [ \"$3\" = --vm ] || exit 9\nprintf 'guest-ready\\n' >&2\nIFS= read -r answer\n[ \"$answer\" = from-terminal ] || exit 8\nprintf 'guest-output:%s\\n' \"$answer\" >&2\nprintf '/host/checkout\\n'\n"), 0700))
			program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu open cards --vm\nprintf 'parent=%s\\n' \"$(pwd -P)\"\n"
			cmd := exec.Command(shell, "-f", "-c", program)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "TEST_PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			cmd.Stdin = strings.NewReader("from-terminal\n")
			var diagnostics bytes.Buffer
			cmd.Stderr = &diagnostics
			result, err := cmd.Output()
			must(t, err)
			canonical, err := filepath.EvalSymlinks(dir)
			must(t, err)
			check(t, string(result) == "/host/checkout\nparent="+canonical+"\n", string(result))
			check(t, diagnostics.String() == "guest-ready\nguest-output:from-terminal\n", diagnostics.String())
		})
	}
}
