package cli

import (
	_ "embed"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func shellInit() *cobra.Command {
	var install bool
	cmd := &cobra.Command{
		Use: "shell-init SHELL", Short: "Print shell integration for bash or zsh", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "bash" && args[0] != "zsh" {
				return fmt.Errorf("unsupported shell %q; use bash or zsh", args[0])
			}
			if install {
				edits, err := prepareShellInstall(args[0])
				if err != nil {
					return err
				}
				if err := previewShellInstall(edits, cmd.ErrOrStderr()); err != nil {
					return err
				}
				if err := applyShellInstall(edits, cmd.ErrOrStderr()); err != nil {
					return err
				}
				return reloadShellInstruction(args[0], cmd.ErrOrStderr())
			}
			_, err := io.WriteString(cmd.OutOrStdout(), shellIntegration)
			return err
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "Install integration in shell startup files")
	return cmd
}

//go:embed scripts/shell-init.sh
var shellIntegration string
