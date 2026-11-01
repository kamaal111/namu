package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/kamaal111/namu/internal/herdr"
	"github.com/kamaal111/namu/internal/integration"
	"github.com/kamaal111/namu/internal/rex"
	"github.com/spf13/cobra"
)

type integrationDefinition struct {
	flag, help string
	detected   func() bool
	hooks      func() integration.Hooks
}

// Register additional technologies here; lifecycle orchestration stays shared.
var integrations = []integrationDefinition{
	{flag: "rex", help: "Use Rex (automatic inside Rex; --rex=false disables)", detected: rex.Detected, hooks: rex.Hooks},
	{flag: "herdr", help: "Use Herdr (automatic inside Herdr; --herdr=false disables)", detected: herdr.Detected, hooks: herdr.Hooks},
}

func registerIntegrations(cmd *cobra.Command) {
	for _, definition := range integrations {
		cmd.Flags().Bool(definition.flag, false, definition.help)
	}
}

func selectedIntegration(cmd *cobra.Command) (integration.Hooks, error) {
	if vmEnabled(cmd) {
		for _, definition := range integrations {
			if cmd.Flags().Lookup(definition.flag) == nil {
				continue
			}
			enabled, _ := cmd.Flags().GetBool(definition.flag)
			if enabled && cmd.Flags().Changed(definition.flag) {
				return integration.Hooks{}, fmt.Errorf("--vm cannot be combined with --%s", definition.flag)
			}
		}
		return integration.Hooks{}, nil
	}
	var selected integration.Hooks
	for _, definition := range integrations {
		if cmd.Flags().Lookup(definition.flag) == nil {
			continue
		}
		enabled, err := cmd.Flags().GetBool(definition.flag)
		if err != nil {
			return integration.Hooks{}, err
		}
		if !cmd.Flags().Changed(definition.flag) {
			enabled = definition.detected()
		}
		if !enabled {
			continue
		}
		if selected.Name != "" {
			return integration.Hooks{}, fmt.Errorf("multiple integrations selected: %s and %s", selected.Name, definition.flag)
		}
		selected = definition.hooks()
	}
	return selected, nil
}

func shellIntegrationHooks() integration.Hooks {
	return integration.Shell(func(path string) error {
		result := os.Getenv("NAMU_SHELL_RESULT")
		if result == "" {
			return nil
		}
		file, err := os.OpenFile(result, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			return fmt.Errorf("cannot report shell navigation: %w", err)
		}
		_, writeErr := fmt.Fprintln(file, path)
		return errors.Join(writeErr, file.Close())
	})
}
