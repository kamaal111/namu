package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/integration"
	"github.com/kamaal111/namu/internal/vm"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"github.com/kamaal111/namu/internal/vm/resources"
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
	registerBoolPair(cmd, "vm", false, "Run setup and open a guest shell in a shared persistent VM", "Use the host worktree without a VM")
	registerCredentialReuse(cmd)
	registerVMResources(cmd)
	cmd.Flags().String("os", "", guestOSHelp())
	cmd.Flags().String("vm-name", "", "Use an existing named namu VM (requires --vm)")
	_ = cmd.RegisterFlagCompletionFunc("os", completeOS)
	_ = cmd.RegisterFlagCompletionFunc("vm-name", completeVM)
}
func completeOS(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return registry.Names(), cobra.ShellCompDirectiveNoFileComp
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
	m := vm.New(data, host.Exec{}, confirm, cmd.InOrStdin(), cmd.ErrOrStderr())
	reuse := true
	if cmd.Flags().Lookup("reuse-credentials") != nil {
		reuse, _ = cmd.Flags().GetBool("reuse-credentials")
	}
	m.NoReuseCredentials = !reuse
	m.Resources = vmResources(cmd)
	if err := m.Resources.Validate(); err != nil {
		return nil, err
	}
	m.SourceDirectory, _ = os.Getwd()
	return m, nil
}
func validateVMOptions(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("vm") == nil {
		return nil
	}
	osName, _ := cmd.Flags().GetString("os")
	name, _ := cmd.Flags().GetString("vm-name")
	if !vmEnabled(cmd) {
		for _, flag := range []string{"os", "vm-name", "vm-cpus", "vm-memory", "vm-disk"} {
			if cmd.Flags().Changed(flag) {
				return fmt.Errorf("VM options require --vm (including --%s)", flag)
			}
		}
		return nil
	}
	if osName != "" || cmd.Flags().Changed("os") {
		if err := vm.ValidateOS(osName); err != nil {
			return err
		}
	}
	if name != "" || cmd.Flags().Changed("vm-name") {
		if err := state.ValidateName(name); err != nil {
			return err
		}
	}
	return vmResources(cmd).Validate()
}

func registerVMResources(cmd *cobra.Command) {
	cmd.Flags().Int("vm-cpus", 0, fmt.Sprintf("VM CPU count (new VM default: %d)", registry.Default().Resources.CPUs))
	cmd.Flags().Int("vm-memory", 0, fmt.Sprintf("VM memory in MiB (new VM default: %d)", registry.Default().Resources.MemoryMB))
	defaults := []string{}
	for _, definition := range registry.All() {
		defaults = append(defaults, fmt.Sprintf("%s %d", definition.Name, definition.Resources.DiskGB))
	}
	cmd.Flags().Int("vm-disk", 0, "VM disk capacity in GB; cannot shrink (new defaults: "+strings.Join(defaults, ", ")+")")
}

func vmResources(cmd *cobra.Command) resources.Options {
	value := func(name string) *int {
		if cmd.Flags().Lookup(name) == nil {
			return nil
		}
		n, _ := cmd.Flags().GetInt(name)
		if n == 0 && !cmd.Flags().Changed(name) {
			return nil
		}
		return &n
	}
	return resources.Options{CPUs: value("vm-cpus"), MemoryMB: value("vm-memory"), DiskGB: value("vm-disk")}
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
			return registry.Default().Name, nil
		}
		answer, err := readTerminalLine(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(), guestOSPrompt())
		if err != nil {
			return "", err
		}
		if answer == "" {
			return registry.Default().Name, nil
		}
		answer = strings.ToLower(answer)
		return answer, vm.ValidateOS(answer)
	}
	s.Environment = &vm.Worktree{Manager: m, Name: name, OS: osName, Default: name == "", ChooseOS: choose}
	s.Integration, s.Fallback = integration.Hooks{}, integration.Hooks{}
	return nil
}
func registerCredentialReuse(cmd *cobra.Command) {
	registerBoolPair(cmd, "reuse-credentials", true, "Reuse supported host agent credentials and Git authentication", "Use guest-only credentials without host access or SSH agent forwarding")
}
func vmCommand(dir string) *cobra.Command {
	root := &cobra.Command{Use: "vm", Short: "Manage namu-owned persistent Tart VMs", Args: cobra.NoArgs}
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd.Flags().Lookup("reuse-credentials") == nil {
			return nil
		}
		if _, err := git.Discover(cmd.Context(), dir); err != nil {
			return nil
		}
		return applyProjectDefaults(cmd, dir)
	}
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
	registerCredentialReuse(create)
	registerVMResources(create)
	create.Flags().StringVar(&osName, "os", registry.Default().Name, guestOSHelp())
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
		}, RunE: func(cmd *cobra.Command, args []string) (result error) {
			m, err := vmManager(cmd)
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, m.Close()) }()
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
		if action == "start" || action == "shell" {
			registerCredentialReuse(cmd)
			registerVMResources(cmd)
		}
		root.AddCommand(cmd)
	}
	return root
}

func guestOSHelp() string {
	choices := []string{}
	for _, definition := range registry.All() {
		choices = append(choices, definition.Name+"="+definition.Label)
	}
	return "Guest OS: " + strings.Join(choices, " or ")
}

func guestOSPrompt() string {
	labels := []string{}
	for _, definition := range registry.All() {
		labels = append(labels, definition.Label)
	}
	return fmt.Sprintf("Default VM OS: %s? [%s; default %s] ", strings.Join(labels, " or "), strings.Join(registry.Names(), "/"), registry.Default().Name)
}
