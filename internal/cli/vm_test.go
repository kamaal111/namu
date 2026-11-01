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
