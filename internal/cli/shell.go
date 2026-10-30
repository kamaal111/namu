package cli

import (
	_ "embed"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func shellInit() *cobra.Command {
	return &cobra.Command{
		Use: "shell-init SHELL", Short: "Print shell integration for bash or zsh", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "bash" && args[0] != "zsh" {
				return fmt.Errorf("unsupported shell %q; use bash or zsh", args[0])
			}
			_, err := io.WriteString(cmd.OutOrStdout(), shellIntegration)
			return err
		},
	}
}

//go:embed scripts/shell-init.sh
var shellIntegration string
