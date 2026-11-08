package cli

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type shellDefinition struct {
	name           string
	writeInit      func(*cobra.Command) error
	prepareInstall func() ([]shellEdit, error)
}

// Register shell-specific policy here; command and installation orchestration stay shared.
var shells = []shellDefinition{bashShellDefinition(), zshShellDefinition(), nuShellDefinition()}

func shellNames() []string {
	names := make([]string, len(shells))
	for i, definition := range shells {
		names[i] = definition.name
	}
	return names
}

func shellChoices() string {
	names := shellNames()
	return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
}

func resolveShell(name string) (shellDefinition, error) {
	for _, definition := range shells {
		if definition.name == name {
			return definition, nil
		}
	}
	return shellDefinition{}, fmt.Errorf("unsupported shell %q; use %s", name, shellChoices())
}

func shellInit() *cobra.Command {
	var install bool
	cmd := &cobra.Command{
		Use: "shell-init SHELL", Short: "Print shell integration for " + shellChoices(), Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return shellNames(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			definition, err := resolveShell(args[0])
			if err != nil {
				return err
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
			return definition.writeInit(cmd)
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "Install integration in shell startup files")
	return cmd
}

//go:embed scripts/shell-init.sh
var shellIntegration string

func supportedShell(shell string) bool {
	_, err := resolveShell(shell)
	return err == nil
}
