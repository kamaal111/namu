package cli

import (
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func zshShellDefinition() shellDefinition {
	return shellDefinition{name: "zsh", writeInit: writeZshShellInit, prepareInstall: prepareZshShellInstall}
}

func writeZshShellInit(cmd *cobra.Command) error {
	if _, err := io.WriteString(cmd.OutOrStdout(), shellIntegration); err != nil {
		return err
	}
	if _, err := io.WriteString(cmd.OutOrStdout(), "\nif ! (( $+functions[compdef] )); then\n  autoload -Uz compinit\n  compinit -i\nfi\n"); err != nil {
		return err
	}
	return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
}

func prepareZshShellInstall() ([]shellEdit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := os.Getenv("ZDOTDIR")
	if dir == "" {
		dir = home
	}
	return prepareEvalShellInstall("zsh", []string{filepath.Join(dir, ".zshrc")}, "command -v namu >/dev/null 2>&1")
}
