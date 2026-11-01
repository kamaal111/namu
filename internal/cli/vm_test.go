package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
	"github.com/kamaal111/namu/internal/vm"
	"github.com/kamaal111/namu/internal/vm/resources"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/spf13/cobra"
)

func TestVMFlagsRequireExplicitOptIn(t *testing.T) {
	repo := testutil.Repository(t)
	for _, args := range [][]string{{"create", "cards", "--base", "main", "--os", "linux"}, {"open", "main", "--vm-name", "sandbox"}, {"setup", "cards", "--os", "macos"}} {
		out, _, err := execute(t, repo.Source, args...)
		check(t, err != nil && strings.Contains(err.Error(), "require --vm"), "must require explicit VM opt-in")
		check(t, out == "", "must not print path")
	}
	check(t, testutil.Git(t, repo.Source, "branch", "--list", "cards") == "", "must not create branch")
}

func TestVMResourceConfigAndFlagPrecedence(t *testing.T) {
	for _, path := range [][]string{{"create"}, {"open"}, {"setup"}, {"vm", "create"}, {"vm", "start"}, {"vm", "shell"}} {
		for _, tc := range []struct {
			name  string
			flags []string
			want  resources.Settings
		}{
			{"config", nil, resources.Settings{CPUs: 2, MemoryMB: 4096, DiskGB: 90}},
			{"flags", []string{"--vm-cpus", "6", "--vm-memory", "12288", "--vm-disk", "120"}, resources.Settings{CPUs: 6, MemoryMB: 12288, DiskGB: 120}},
		} {
			t.Run(strings.Join(path, "-")+"/"+tc.name, func(t *testing.T) {
				repo := testutil.Repository(t)
				testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[vm]\nos = 'linux'\nname = 'sandbox'\ncpus = 2\nmemory_mb = 4096\ndisk_gb = 90\n")
				cmd, _, err := New(repo.Source).Find(path)
				must(t, err)
				cmd.SetContext(context.Background())
				must(t, cmd.ParseFlags(tc.flags))
				must(t, applyProjectDefaults(cmd, repo.Source))
				check(t, vmResources(cmd).Apply(resources.Settings{}) == tc.want, "unexpected effective resources")
				check(t, !cmd.Flags().Changed("reuse-credentials"), "project defaults must not appear explicit")
			})
		}
	}
}

func TestVMSelectionConfigRespectsFlags(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      []string
		os, vmName string
	}{
		{"config", nil, "linux", "sandbox"},
		{"override", []string{"--os", "macos", "--vm-name", "other"}, "macos", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[vm]\nos = 'linux'\nname = 'sandbox'\n")
			cmd, _, err := New(repo.Source).Find([]string{"create"})
			must(t, err)
			cmd.SetContext(context.Background())
			must(t, cmd.ParseFlags(tc.flags))
			must(t, applyProjectDefaults(cmd, repo.Source))
			osName, err := cmd.Flags().GetString("os")
			must(t, err)
			name, err := cmd.Flags().GetString("vm-name")
			must(t, err)
			check(t, osName == tc.os && name == tc.vmName, "unexpected VM selection")
			check(t, !vmEnabled(cmd), "settings must not enable VM execution")
		})
	}
}

func TestVMResourceFlagsFailBeforeWorktreeMutation(t *testing.T) {
	for _, flags := range [][]string{
		{"--vm-cpus", "2"}, {"--vm-memory", "4096"}, {"--vm-disk", "80"},
		{"--vm", "--vm-cpus", "0"}, {"--vm", "--vm-memory", "-1"}, {"--vm", "--vm-disk", "65536"},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			repo := testutil.Repository(t)
			out, _, err := execute(t, repo.Source, append([]string{"create", "cards", "--base", "main", "--no-focus"}, flags...)...)
			check(t, err != nil, "invalid VM resource flags must fail")
			check(t, out == "", "must not print path")
			check(t, testutil.Git(t, repo.Source, "branch", "--list", "cards") == "", "must not create branch")
		})
	}
}

func TestVMResourceConfigDoesNotProvisionWithoutVMOptIn(t *testing.T) {
	repo := testutil.Repository(t)
	t.Setenv("HERDR_ENV", "")
	t.Setenv("REX_SESSION", "")
	testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[vm]\nos = 'linux'\nname = 'sandbox'\ncpus = 2\nmemory_mb = 4096\ndisk_gb = 90\n")
	out, _, err := execute(t, repo.Source, "create", "cards", "--base", "main")
	must(t, err)
	check(t, out == filepath.Join(repo.Main, ".agents/worktrees/cards")+"\n", out)
}
func TestVMRejectsExplicitTerminalIntegrationAndIgnoresDetection(t *testing.T) {
	t.Setenv("REX_SESSION", "detected-session")
	t.Setenv("HERDR_ENV", "1")
	root := New(t.TempDir())
	cmd, _, err := root.Find([]string{"open"})
	must(t, err)
	must(t, cmd.Flags().Set("vm", "true"))
	selected, err := selectedIntegration(cmd)
	must(t, err)
	check(t, selected.Name == "", "VM must bypass detected terminals")
	must(t, cmd.Flags().Set("rex", "true"))
	_, err = selectedIntegration(cmd)
	check(t, err != nil && strings.Contains(err.Error(), "--rex"), "must reject explicit combination")
}
func TestVMSelectorsAreValidatedBeforeMutation(t *testing.T) {
	repo := testutil.Repository(t)
	for _, flags := range [][]string{{"--os", "windows"}, {"--os", ""}, {"--vm-name", "../unsafe"}, {"--vm-name", ""}} {
		args := append([]string{"create", "cards", "--base", "main", "--vm", "--no-focus"}, flags...)
		out, _, err := execute(t, repo.Source, args...)
		check(t, err != nil, "invalid selector must fail")
		check(t, out == "", "invalid selector must not print path")
	}
	check(t, testutil.Git(t, repo.Source, "branch", "--list", "cards") == "", "must not create branch")
}
func TestVMNoninteractiveShellRequiresNoFocus(t *testing.T) {
	repo := testutil.Repository(t)
	out, _, err := execute(t, repo.Source, "create", "cards", "--base", "main", "--vm")
	check(t, err != nil && strings.Contains(err.Error(), "--no-focus"), "must explain noninteractive preparation")
	check(t, out == "", "must fail before printing")
}
func TestVMListAndCompletionOutsideRepositoryDoNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	out, _, err := execute(t, t.TempDir(), "vm", "list")
	must(t, err)
	check(t, strings.Contains(out, "NAME") && !strings.Contains(out, "namu-macos"), out)
	names, directive := completeVM(nil, nil, "")
	check(t, len(names) == 0 && directive == cobra.ShellCompDirectiveNoFileComp, "must complete without VM access")
	_, err = os.Stat(filepath.Join(home, ".local"))
	check(t, os.IsNotExist(err), "read-only commands must not initialize state")
}
func TestVMNameCompletionReadsOnlyRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	data, err := vm.DefaultData()
	must(t, err)
	store := state.Store{Data: data}
	unlock, err := store.Lock(context.Background())
	must(t, err)
	must(t, store.Write(state.Registry{Version: 1, VMs: []state.VM{{ID: "test", Name: "sandbox", OS: "linux", Image: "pinned", Stage: "ready"}}}))
	unlock()
	names, directive := completeVM(nil, nil, "sand")
	check(t, reflect.DeepEqual(names, []string{"sandbox"}), "must complete owned names")
	check(t, directive == cobra.ShellCompDirectiveNoFileComp, "must disable file completion")
}

func TestVMAndCredentialDefaultsRespectFlagPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     []string
		vm, reuse bool
	}{
		{name: "config", vm: true, reuse: false},
		{name: "disable-vm", flags: []string{"--vm=false"}, vm: false, reuse: false},
		{name: "reuse-override", flags: []string{"--reuse-credentials"}, vm: true, reuse: true},
		{name: "negative-alias", flags: []string{"--no-reuse-credentials"}, vm: true, reuse: false},
		{name: "false-value", flags: []string{"--reuse-credentials=false"}, vm: true, reuse: false},
		{name: "last-flag-wins", flags: []string{"--no-reuse-credentials", "--reuse-credentials"}, vm: true, reuse: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			must(t, os.WriteFile(filepath.Join(repo.Source, ".namu.toml"), []byte("[defaults]\nvm = true\nreuse_credentials = false\n"), 0600))
			root := New(repo.Source)
			cmd, _, err := root.Find([]string{"create"})
			must(t, err)
			cmd.SetContext(context.Background())
			must(t, cmd.ParseFlags(tc.flags))
			cmd.SetContext(context.Background())
			must(t, applyProjectDefaults(cmd, repo.Source))
			enabled, err := cmd.Flags().GetBool("reuse-credentials")
			must(t, err)
			check(t, vmEnabled(cmd) == tc.vm, "unexpected VM selection")
			check(t, enabled == tc.reuse, "unexpected credential selection")
		})
	}
}
func TestCreateWithVMConfigDisabledStillCreatesOnlyHostWorktree(t *testing.T) {
	repo := testutil.Repository(t)
	must(t, os.WriteFile(filepath.Join(repo.Source, ".namu.toml"), []byte("[defaults]\nvm = false\nreuse_credentials = true\n"), 0600))
	t.Setenv("HERDR_ENV", "")
	t.Setenv("REX_SESSION", "")
	out, _, err := execute(t, repo.Source, "create", "cards", "--base", "main")
	must(t, err)
	check(t, out == filepath.Join(repo.Main, ".agents/worktrees/cards")+"\n", out)
}
func TestBuiltInVMAndCredentialDefaults(t *testing.T) {
	repo := testutil.Repository(t)
	root := New(repo.Source)
	cmd, _, err := root.Find([]string{"create"})
	must(t, err)
	cmd.SetContext(context.Background())
	must(t, applyProjectDefaults(cmd, repo.Source))
	reuse, err := cmd.Flags().GetBool("reuse-credentials")
	must(t, err)
	check(t, !vmEnabled(cmd) && reuse, "VM must be opt-in and host reuse default on")
}

func TestExplicitVMOptOutOverridesEnabledProjectConfigWithoutProvisioning(t *testing.T) {
	for _, flag := range []string{"--vm=false", "--no-vm"} {
		t.Run(flag, func(t *testing.T) {
			repo := testutil.Repository(t)
			t.Setenv("HERDR_ENV", "")
			t.Setenv("REX_SESSION", "")
			must(t, os.WriteFile(filepath.Join(repo.Source, ".namu.toml"), []byte("[defaults]\nvm = true\n"), 0600))
			out, _, err := execute(t, repo.Source, "create", "cards", "--base", "main", flag)
			must(t, err)
			check(t, out == filepath.Join(repo.Main, ".agents/worktrees/cards")+"\n", out)
		})
	}
}

func TestCredentialFlagsNeverEnableVMForHostWorktree(t *testing.T) {
	for _, flag := range []string{"--reuse-credentials", "--reuse-credentials=false", "--no-reuse-credentials"} {
		t.Run(flag, func(t *testing.T) {
			repo := testutil.Repository(t)
			t.Setenv("HERDR_ENV", "")
			t.Setenv("REX_SESSION", "")
			out, _, err := execute(t, repo.Source, "create", "cards", "--base", "main", flag)
			must(t, err)
			check(t, out == filepath.Join(repo.Main, ".agents/worktrees/cards")+"\n", out)
		})
	}
}
