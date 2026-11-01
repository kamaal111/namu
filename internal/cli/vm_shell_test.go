package cli

import (
	"bytes"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/vm-command.sh
var vmCommandFixture string

//go:embed testdata/vm-shell-prefix.sh
var vmShellPrefix string

//go:embed testdata/vm-shell-run.sh
var vmShellRun string

func TestVMShellWrapperKeepsTerminalStreamsAndParentDirectory(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			dir := t.TempDir()
			executable := filepath.Join(dir, "namu")
			must(t, os.WriteFile(executable, []byte(vmCommandFixture), 0700))
			program := vmShellPrefix + shellIntegration + "\n" + vmShellRun
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
