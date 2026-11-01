package vm

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/kamaal111/namu/internal/vm/tart"
)

type commandFake struct {
	commands               []host.Command
	vms                    map[string]tart.VM
	failClone              bool
	failMount              bool
	failSSH                bool
	authorized             bool
	starts, stops, deletes int
}

func testKey() string {
	b := make([]byte, 51)
	binary.BigEndian.PutUint32(b[:4], 11)
	copy(b[4:15], "ssh-ed25519")
	binary.BigEndian.PutUint32(b[15:19], 32)
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(b)
}
func (f *commandFake) Run(_ context.Context, c host.Command) ([]byte, error) {
	f.commands = append(f.commands, c)
	switch c.Name {
	case "sw_vers":
		return []byte("27.2"), nil
	case "ssh-keygen":
		if c.Args[0] == "-y" {
			return []byte(testKey()), nil
		}
		return nil, os.WriteFile(c.Args[len(c.Args)-1], []byte("test-only identity"), 0600)
	case "ssh":
		if c.Args[0] == "-V" {
			return []byte("OpenSSH_10.3p1"), nil
		}
		if !f.authorized {
			if c.Err != nil {
				io.WriteString(c.Err, "Permission denied (publickey)")
			}
			return nil, errors.New("key reset by guest boot")
		}
		if f.failSSH {
			return nil, errors.New("guest command failed")
		}
		return nil, nil
	}
	switch c.Args[0] {
	case "--version":
		return []byte("2.41.1"), nil
	case "list":
		rows := []tart.VM{}
		for _, v := range f.vms {
			rows = append(rows, v)
		}
		return json.Marshal(rows)
	case "clone":
		if f.failClone {
			return nil, errors.New("clone failed")
		}
		name := c.Args[len(c.Args)-1]
		f.vms[name] = tart.VM{Name: name}
		return nil, nil
	case "set":
		return nil, nil
	case "stop":
		f.stops++
		v := f.vms[c.Args[1]]
		v.Running = false
		f.vms[v.Name] = v
		return nil, nil
	case "delete":
		f.deletes++
		delete(f.vms, c.Args[1])
		return nil, nil
	case "ip":
		return []byte("192.0.2.1"), nil
	case "exec":
		script, _ := io.ReadAll(c.In)
		if strings.Contains(string(script), "public_key=") {
			f.authorized = true
			return []byte(testKey()), nil
		}
		if f.failMount && strings.Contains(string(script), "/var/db/namu-vm") {
			return nil, errors.New("mount failed")
		}
		return nil, nil
	}
	return nil, errors.New("unexpected command " + c.Name + " " + strings.Join(c.Args, " "))
}
func (f *commandFake) Detach(_ context.Context, c host.Command, _ string) error {
	f.commands = append(f.commands, c)
	f.starts++
	f.authorized = false
	name := c.Args[len(c.Args)-1]
	f.vms[name] = tart.VM{Name: name, Running: true}
	return nil
}
func managerFixture(t *testing.T) (*Manager, *commandFake) {
	t.Helper()
	f := &commandFake{vms: map[string]tart.VM{}}
	m := New(t.TempDir(), f, func(context.Context, string) error { return nil }, strings.NewReader(""), io.Discard)
	m.Tart.Host = func() (string, string) { return "darwin", "arm64" }
	return m, f
}
func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func assert(t *testing.T, value bool, message any) {
	t.Helper()
	if !value {
		t.Fatal(message)
	}
}
func readyVM(t *testing.T, m *Manager, f *commandFake, roots []string) state.VM {
	t.Helper()
	v, err := m.Ensure(context.Background(), "sandbox", "linux", roots, true)
	require(t, err)
	assert(t, f.starts == 1, f.starts)
	return v
}
func TestEnsureCreatesAndReusesVMWithoutPromptOrRestart(t *testing.T) {
	m, f := managerFixture(t)
	root := t.TempDir()
	v := readyVM(t, m, f, []string{root})
	m.Confirm = func(context.Context, string) error { return errors.New("unexpected confirmation") }
	reused, err := m.Ensure(context.Background(), "sandbox", "linux", []string{root}, false)
	require(t, err)
	assert(t, v.ID == reused.ID && reused.Stage == "ready", reused)
	assert(t, f.starts == 1 && f.stops == 0, f.commands)
	assert(t, reflect.DeepEqual(reused.Applied, []string{root}), reused.Applied)
}
func TestEnsureDeclinedCreationLeavesNoRegistryOrIdentity(t *testing.T) {
	m, f := managerFixture(t)
	m.Confirm = func(context.Context, string) error { return errors.New("declined") }
	_, err := m.Ensure(context.Background(), "namu-macos", "macos", nil, true)
	assert(t, err != nil, err)
	_, registryErr := os.Stat(filepath.Join(m.Store.Data, "registry.json"))
	_, keyErr := os.Stat(filepath.Join(m.Store.Data, "ssh", "id_ed25519"))
	assert(t, os.IsNotExist(registryErr) && os.IsNotExist(keyErr), "decline must not provision")
	assert(t, f.starts == 0 && len(f.vms) == 0, f.commands)
}
func TestEnsureDeclinedRestartPreservesRunningVMAndShares(t *testing.T) {
	m, f := managerFixture(t)
	root := t.TempDir()
	v := readyVM(t, m, f, []string{root})
	m.Confirm = func(context.Context, string) error { return errors.New("declined") }
	_, err := m.Ensure(context.Background(), v.Name, "", []string{t.TempDir()}, false)
	assert(t, err != nil, err)
	r, readErr := m.Store.Read()
	require(t, readErr)
	assert(t, reflect.DeepEqual(r.VMs[0].Shares, []string{root}), r)
	assert(t, f.stops == 0 && f.vms[v.Name].Running, f.commands)
}
func TestEnsureApprovedRestartAddsShareAndRetainsExistingShare(t *testing.T) {
	m, f := managerFixture(t)
	first := t.TempDir()
	v := readyVM(t, m, f, []string{first})
	second := t.TempDir()
	updated, err := m.Ensure(context.Background(), v.Name, "", []string{second}, false)
	require(t, err)
	expected, err := ShareRoots([]string{first, second})
	require(t, err)
	assert(t, reflect.DeepEqual(updated.Applied, expected), updated)
	assert(t, f.stops == 1 && f.starts == 2, f.commands)
}
func TestEnsureCloneFailureRetainsIntentAndRetryUsesSameIdentity(t *testing.T) {
	m, f := managerFixture(t)
	f.failClone = true
	failed, err := m.Ensure(context.Background(), "sandbox", "linux", nil, true)
	assert(t, err != nil, err)
	r, readErr := m.Store.Read()
	require(t, readErr)
	assert(t, r.VMs[0].Stage == "creating" && r.VMs[0].ID == failed.ID, r)
	f.failClone = false
	recovered, err := m.Start(context.Background(), "sandbox")
	require(t, err)
	assert(t, recovered.ID == failed.ID && recovered.Stage == "ready", recovered)
}
func TestEnsureRejectsUnownedNameCollision(t *testing.T) {
	m, f := managerFixture(t)
	f.vms["sandbox"] = tart.VM{Name: "sandbox"}
	_, err := m.Ensure(context.Background(), "sandbox", "linux", nil, true)
	assert(t, err != nil && strings.Contains(err.Error(), "adoption"), err)
	r, readErr := m.Store.Read()
	require(t, readErr)
	assert(t, len(r.VMs) == 0 && f.starts == 0, r)
}
func TestEnsureRefusesMissingPreviouslyReadyVM(t *testing.T) {
	m, f := managerFixture(t)
	v := readyVM(t, m, f, nil)
	delete(f.vms, v.Name)
	_, err := m.Start(context.Background(), v.Name)
	assert(t, err != nil && strings.Contains(err.Error(), "refusing replacement"), err)
	assert(t, f.starts == 1, f.commands)
}
func TestEnsureRejectsNamedOSConflict(t *testing.T) {
	m, f := managerFixture(t)
	v := readyVM(t, m, f, nil)
	_, err := m.Ensure(context.Background(), v.Name, "macos", nil, false)
	assert(t, err != nil && strings.Contains(err.Error(), "incompatible"), err)
	assert(t, f.starts == 1, f.commands)
}
func TestEnsureMountFailurePreservesReadyVM(t *testing.T) {
	m, f := managerFixture(t)
	f.failMount = true
	v, err := m.Ensure(context.Background(), "sandbox", "linux", []string{t.TempDir()}, true)
	assert(t, err != nil && strings.Contains(err.Error(), "mounting failed"), err)
	assert(t, v.Stage == "ready" && f.deletes == 0, v)
}
func TestEnsureUnsupportedHostDoesNotProvision(t *testing.T) {
	m, f := managerFixture(t)
	m.Tart.Host = func() (string, string) { return "linux", "arm64" }
	_, err := m.Ensure(context.Background(), "sandbox", "linux", nil, true)
	assert(t, err != nil && strings.Contains(err.Error(), "Apple Silicon"), err)
	assert(t, len(f.commands) == 0, f.commands)
}
func TestDeleteRequiresStoppedOwnedVMAndConfirmation(t *testing.T) {
	m, f := managerFixture(t)
	v := readyVM(t, m, f, nil)
	err := m.Delete(context.Background(), v.Name)
	assert(t, err != nil && strings.Contains(err.Error(), "stop VM"), err)
	require(t, m.Stop(context.Background(), v.Name))
	m.Confirm = func(context.Context, string) error { return errors.New("declined") }
	err = m.Delete(context.Background(), v.Name)
	assert(t, err != nil && f.deletes == 0, err)
	m.Confirm = func(context.Context, string) error { return nil }
	require(t, m.Delete(context.Background(), v.Name))
	r, readErr := m.Store.Read()
	require(t, readErr)
	assert(t, len(r.VMs) == 0 && f.deletes == 1, r)
}
func TestUnknownNamedVMDoesNotCreate(t *testing.T) {
	m, f := managerFixture(t)
	_, err := m.Ensure(context.Background(), "missing", "", nil, false)
	assert(t, err != nil && strings.Contains(err.Error(), "namu vm create"), err)
	assert(t, len(f.commands) == 0, f.commands)
}
func TestConcurrentEnsureCreatesOneVM(t *testing.T) {
	m, f := managerFixture(t)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := m.Ensure(context.Background(), "sandbox", "linux", nil, true); failures <- err })
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require(t, err)
	}
	assert(t, f.starts == 1, f.commands)
	r, err := m.Store.Read()
	require(t, err)
	assert(t, len(r.VMs) == 1, r)
}
func TestCancelledEnsureDoesNotProvision(t *testing.T) {
	m, f := managerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.Ensure(ctx, "sandbox", "linux", nil, true)
	assert(t, errors.Is(err, context.Canceled), err)
	assert(t, f.starts == 0, f.commands)
}

func TestWorktreeFirstUseChoosesLinuxAndSubsequentExplicitLinuxReuses(t *testing.T) {
	m, f := managerFixture(t)
	repo := testutil.Repository(t)
	selected := 0
	w := &Worktree{Manager: m, Default: true, ChooseOS: func(context.Context) (string, error) { selected++; return "linux", nil }}
	require(t, w.Prepare(context.Background(), repo, filepath.Join(repo.Main, ".agents/worktrees/cards")))
	assert(t, w.selected.Name == "namu-linux" && selected == 1, w.selected)
	explicit := &Worktree{Manager: m, Default: true, OS: "linux"}
	require(t, explicit.Prepare(context.Background(), repo, filepath.Join(repo.Main, ".agents/worktrees/other")))
	assert(t, explicit.selected.ID == w.selected.ID && f.starts == 1, explicit.selected)
}

func TestRunQuotesGuestArgvAndDoesNotForwardInput(t *testing.T) {
	m, f := managerFixture(t)
	v := readyVM(t, m, f, nil)
	path := "/Users/ada/project with space"
	argv := []string{"tool", "a'b", "$(touch /host/file)"}
	require(t, m.Run(context.Background(), v, path, argv, io.Discard))
	last := f.commands[len(f.commands)-1]
	script := "cd " + host.Quote(path) + " && exec " + host.Join(argv)
	escaped := host.Quote(script)
	assert(t, strings.Contains(last.Args[len(last.Args)-1], escaped[1:len(escaped)-1]), last.Args)
	assert(t, last.In == nil, "setup must receive no stdin")
}

func TestRestartRestoresDedicatedKeyAfterGuestBootResetsAuthorization(t *testing.T) {
	m, f := managerFixture(t)
	v := readyVM(t, m, f, nil)
	require(t, m.Stop(context.Background(), v.Name))
	restarted, err := m.Start(context.Background(), v.Name)
	require(t, err)
	assert(t, f.authorized && f.starts == 2 && restarted.ID == v.ID, restarted)
}
