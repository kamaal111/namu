package cli

import (
	"fmt"
	"strconv"

	"github.com/kamaal111/namu/internal/config"
	git "github.com/kamaal111/namu/internal/git"
	"github.com/spf13/cobra"
)

func prepareNavigation(cmd *cobra.Command, dir string, args []string) error {
	if err := cmd.ValidateFlagGroups(); err != nil {
		return err
	}
	if cmd.Name() == "open" && len(args) == 0 && !interactive(cmd) {
		return fmt.Errorf("interactive terminal required; supply a branch: namu open feature/cards")
	}
	for _, name := range []string{"base", "remote"} {
		if cmd.Flags().Changed(name) {
			value, err := cmd.Flags().GetString(name)
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--%s cannot be empty", name)
			}
		}
	}
	if err := applyProjectDefaults(cmd, dir); err != nil {
		return err
	}
	return autoShellInstall(cmd, args)
}

func applyProjectDefaults(cmd *cobra.Command, dir string) error {
	repo, err := git.Discover(cmd.Context(), dir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(repo.Source)
	if err != nil {
		return err
	}
	for _, setting := range []struct {
		name     string
		value    *bool
		fallback bool
	}{
		{"focus", cfg.Defaults.Focus, true},
		{"pr", cfg.Defaults.PR, true},
		{"vm", cfg.Defaults.VM, false},
		{"reuse-credentials", cfg.Defaults.ReuseCredentials, true},
	} {
		flag := cmd.Flags().Lookup(setting.name)
		if flag == nil || cmd.Flags().Changed(setting.name) || cmd.Flags().Changed("no-"+setting.name) {
			continue
		}
		enabled := setting.fallback
		if setting.value != nil {
			enabled = *setting.value
		}
		// Keep project defaults separate from explicitly supplied CLI flags.
		if err := flag.Value.Set(strconv.FormatBool(enabled)); err != nil {
			return err
		}
	}
	for _, setting := range []struct{ name, value string }{
		{"os", cfg.VM.OS}, {"vm-name", cfg.VM.Name},
	} {
		flag := cmd.Flags().Lookup(setting.name)
		if flag != nil && !flag.Changed && setting.value != "" {
			if err := flag.Value.Set(setting.value); err != nil {
				return err
			}
		}
	}
	for _, setting := range []struct {
		name  string
		value *int
	}{
		{"vm-cpus", cfg.VM.CPUs}, {"vm-memory", cfg.VM.MemoryMB}, {"vm-disk", cfg.VM.DiskGB},
	} {
		flag := cmd.Flags().Lookup(setting.name)
		if flag != nil && !flag.Changed && setting.value != nil {
			if err := flag.Value.Set(strconv.Itoa(*setting.value)); err != nil {
				return err
			}
		}
	}
	return nil
}
