package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/kamaal111/namu/internal/integration"
	"github.com/kamaal111/namu/internal/vm"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/kamaal111/namu/internal/worktree"
	"github.com/spf13/cobra"
)

func vmEnabled(cmd *cobra.Command) bool {
	if cmd.Flags().Lookup("vm") == nil {
		return false
	}
	enabled, _ := cmd.Flags().GetBool("vm")
	return enabled
}
func registerVM(cmd *cobra.Command) {
	cmd.Flags().Bool("vm", false, "Run setup and open a guest shell in a shared persistent VM")
	cmd.Flags().String("os", "", "Guest OS: macos (macOS 27) or linux (Ubuntu 24.04)")
	cmd.Flags().String("vm-name", "", "Use an existing named namu VM (requires --vm)")
	_ = cmd.RegisterFlagCompletionFunc("os", completeOS)
	_ = cmd.RegisterFlagCompletionFunc("vm-name", completeVM)
}
func completeOS(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"macos", "linux"}, cobra.ShellCompDirectiveNoFileComp
}
func completeVM(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	data, err := vm.DefaultData()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	r, err := (state.Store{Data: data}).Read()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := []string{}
	for _, v := range r.VMs {
		if strings.HasPrefix(v.Name, prefix) {
			names = append(names, v.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
func vmManager(cmd *cobra.Command) (*vm.Manager, error) {
	data, err := vm.DefaultData()
	if err != nil {
		return nil, err
	}
	confirm := func(ctx context.Context, summary string) error {
		if !interactive(cmd) {
			return fmt.Errorf("interactive terminal required to confirm VM changes; rerun in a terminal\n%s", summary)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), summary)
		answer, err := readTerminalLine(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(), "Continue? [y/N] ")
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			return fmt.Errorf("VM operation declined")
		}
		return nil
	}
	return vm.New(data, host.Exec{}, confirm, cmd.InOrStdin(), cmd.ErrOrStderr()), nil
}
func validateVMOptions(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("vm") == nil {
		return nil
	}
	osName, _ := cmd.Flags().GetString("os")
	name, _ := cmd.Flags().GetString("vm-name")
	if !vmEnabled(cmd) {
		if cmd.Flags().Changed("os") || cmd.Flags().Changed("vm-name") {
			return fmt.Errorf("--os and --vm-name require --vm")
		}
		return nil
	}
	if cmd.Flags().Changed("os") {
		if err := vm.ValidateOS(osName); err != nil {
			return err
		}
	}
	if cmd.Flags().Changed("vm-name") {
		if err := state.ValidateName(name); err != nil {
			return err
		}
	}
	return nil
}
func configureVM(cmd *cobra.Command, s *worktree.Service) error {
	if err := validateVMOptions(cmd); err != nil {
		return err
	}
	if !vmEnabled(cmd) {
		return nil
	}
	osName, _ := cmd.Flags().GetString("os")
	name, _ := cmd.Flags().GetString("vm-name")
	if cmd.Name() != "setup" && !s.NoFocus && !interactive(cmd) {
		return fmt.Errorf("interactive terminal required for a VM shell; use --no-focus to prepare without attaching")
	}
	m, err := vmManager(cmd)
	if err != nil {
		return err
	}
	choose := func(ctx context.Context) (string, error) {
		if !interactive(cmd) {
			return "macos", nil
		}
		answer, err := readTerminalLine(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(), "Default VM OS: macOS 27 or Linux (Ubuntu 24.04)? [macos/linux; default macos] ")
		if err != nil {
			return "", err
		}
		if answer == "" {
			return "macos", nil
		}
		answer = strings.ToLower(answer)
		return answer, vm.ValidateOS(answer)
	}
	s.Environment = &vm.Worktree{Manager: m, Name: name, OS: osName, Default: name == "", ChooseOS: choose}
	s.Integration, s.Fallback = integration.Hooks{}, integration.Hooks{}
	return nil
}
func vmCommand() *cobra.Command {
	root := &cobra.Command{Use: "vm", Short: "Manage namu-owned persistent Tart VMs", Args: cobra.NoArgs}
	var osName string
	create := &cobra.Command{Use: "create NAME", Short: "Confirm and prepare a named VM", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := vm.ValidateOS(osName); err != nil {
			return err
		}
		m, err := vmManager(cmd)
		if err != nil {
			return err
		}
		v, err := m.Create(cmd.Context(), args[0], osName)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), v.Name)
		return err
	}}
	create.Flags().StringVar(&osName, "os", "macos", "Guest OS: macos or linux")
	_ = create.RegisterFlagCompletionFunc("os", completeOS)
	list := &cobra.Command{Use: "list", Short: "List namu-owned VMs and live status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		m, err := vmManager(cmd)
		if err != nil {
			return err
		}
		rows, err := m.List(cmd.Context())
		if err != nil {
			return err
		}
		out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(out, "NAME\tOS\tSTATE")
		for _, row := range rows {
			fmt.Fprintf(out, "%s\t%s\t%s\n", row.Name, row.OS, row.State)
		}
		return out.Flush()
	}}
	root.AddCommand(create, list)
	for _, action := range []string{"start", "stop", "shell", "delete"} {
		cmd := &cobra.Command{Use: action + " NAME", Short: action + " a namu-owned VM", Args: cobra.ExactArgs(1), ValidArgsFunction: func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeVM(cmd, args, prefix)
		}, RunE: func(cmd *cobra.Command, args []string) error {
			m, err := vmManager(cmd)
			if err != nil {
				return err
			}
			switch action {
			case "stop":
				return m.Stop(cmd.Context(), args[0])
			case "delete":
				return m.Delete(cmd.Context(), args[0])
			case "start":
				_, err = m.Start(cmd.Context(), args[0])
				return err
			case "shell":
				if !interactive(cmd) {
					return fmt.Errorf("interactive terminal required for a VM shell")
				}
				v, err := m.Start(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return m.Shell(cmd.Context(), v, "")
			}
			return fmt.Errorf("unsupported VM operation %s", action)
		}}
		root.AddCommand(cmd)
	}
	return root
}
