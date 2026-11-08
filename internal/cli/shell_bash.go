package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func bashShellDefinition() shellDefinition {
	return shellDefinition{name: "bash", writeInit: writeBashShellInit, prepareInstall: prepareBashShellInstall}
}

func writeBashShellInit(cmd *cobra.Command) error {
	if _, err := io.WriteString(cmd.OutOrStdout(), shellIntegration); err != nil {
		return err
	}
	return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
}

func prepareBashShellInstall() ([]shellEdit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	login := filepath.Join(home, ".bash_profile")
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		path := filepath.Join(home, name)
		_, err := os.Lstat(path)
		if err == nil {
			login = path
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return prepareEvalShellInstall("bash", []string{filepath.Join(home, ".bashrc"), login}, "[ -n \"${BASH_VERSION-}\" ] && command -v namu >/dev/null 2>&1")
}
