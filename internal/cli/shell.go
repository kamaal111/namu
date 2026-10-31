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
		ValidArgsFunction: func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return []string{"bash", "zsh"}, cobra.ShellCompDirectiveNoFileComp
		},
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
			if _, err := io.WriteString(cmd.OutOrStdout(), shellIntegration); err != nil {
				return err
			}
			if args[0] == "bash" {
				return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), "\nif ! (( $+functions[compdef] )); then\n  autoload -Uz compinit\n  compinit -i\nfi\n"); err != nil {
				return err
			}
			return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "Install integration in shell startup files")
	return cmd
}

//go:embed scripts/shell-init.sh
var shellIntegration string
