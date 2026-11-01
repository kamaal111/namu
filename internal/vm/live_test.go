package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/kamaal111/namu/internal/worktree"
)

func liveName(t *testing.T) string {
	t.Helper()
	name := os.Getenv("NAMU_VM_SMOKE")
	if name == "" {
		t.Skip("opt-in: just vm-smoke NAME against a prepared, stopped namu VM")
	}
	return name
}
func TestLiveVMWorktree(t *testing.T) {
	name := liveName(t)
	data, err := DefaultData()
	require(t, err)
	m := New(data, host.Exec{}, func(context.Context, string) error {
		return errors.New("live smoke requires a prepared, stopped VM; no implicit creation or restart")
	}, nil, os.Stderr)
	original, err := m.Store.Read()
	require(t, err)
	index, err := state.Find(original, name)
	require(t, err)
	saved := original.VMs[index]
	t.Cleanup(func() {
		require(t, m.Stop(context.Background(), name))
		unlock, err := m.Store.Lock(context.Background())
		require(t, err)
		defer unlock()
		current, err := m.Store.Read()
		require(t, err)
		index, err := state.Find(current, name)
		require(t, err)
		current.VMs[index].Shares = append([]string(nil), saved.Shares...)
		current.VMs[index].Applied = nil
		require(t, m.Store.Write(current))
	})
	repo := testutil.Repository(t)
	testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'printf guest >> tracked.txt; git add tracked.txt; git -c user.name=Smoke -c user.email=smoke@example.com commit -qm guest; uname -s > guest-platform']\n")
	testutil.Git(t, repo.Source, "add", ".")
	testutil.Git(t, repo.Source, "commit", "-qm", "setup")
	var out bytes.Buffer
	environment := &Worktree{Manager: m, Name: name}
	s := worktree.Service{Repo: repo, Out: &out, Err: os.Stderr, NoFocus: true, Environment: environment}
	require(t, s.Create(context.Background(), "smoke/cards", "main", ""))
	path := filepath.Join(repo.Main, ".agents/worktrees/smoke/cards")
	assert(t, out.String() == path+"\n", out.String())
	b, err := os.ReadFile(filepath.Join(path, "tracked.txt"))
	require(t, err)
	assert(t, string(b) == "initialguest", string(b))
	platform, err := os.ReadFile(filepath.Join(path, "guest-platform"))
	require(t, err)
	expected := map[string]string{"linux": "Linux", "macos": "Darwin"}[saved.OS]
	assert(t, strings.TrimSpace(string(platform)) == expected, string(platform))
	assert(t, testutil.Git(t, path, "log", "-1", "--format=%s") == "guest", "guest Git commit must be visible on host")
	require(t, environment.Run(context.Background(), path, []string{"sh", "-c", "test \"$(cat tracked.txt)\" = initialguest"}, io.Discard))
	mounts, err := m.Tart.ExecRoot(context.Background(), name, "export PATH=/usr/bin:/bin:/usr/sbin:/sbin; mount | grep -i virtiofs\n", nil)
	require(t, err)
	t.Log(string(mounts))
	binary := filepath.Join(t.TempDir(), "namu")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/namu")
	buildOutput, err := build.CombinedOutput()
	assert(t, err == nil, string(buildOutput))
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			cmd := exec.Command("python3", "-c", liveShellProgram, binary, shell, repo.Main, name)
			output, err := cmd.CombinedOutput()
			assert(t, err == nil, string(output))
			var result struct {
				Output, Stdout string
				Code           int
			}
			require(t, json.Unmarshal(output, &result))
			assert(t, result.Code == 0, result.Output)
			assert(t, strings.Contains(result.Output, "NAMU_GUEST="+expected), result.Output)
			assert(t, result.Stdout == path+"\nparent="+repo.Main+"\n", result.Stdout)
		})
	}

}

const liveShellProgram = `
import errno, json, os, pty, select, subprocess, sys, time
binary, shell, source, name = sys.argv[1:]
master, slave = pty.openpty()
env = dict(os.environ, TERM="xterm-256color", REX_SESSION="validation-pane", HERDR_ENV="1")
env["PATH"] = os.path.dirname(binary) + os.pathsep + env["PATH"]
program = 'eval "$(namu shell-init ' + shell + ')"; namu open smoke/cards --vm --vm-name ' + name + '; result=$?; printf "parent=%s\n" "$(pwd -P)"; exit "$result"'
proc = subprocess.Popen([shell, "-f", "-c", program], cwd=source, env=env,
    stdin=slave, stdout=subprocess.PIPE, stderr=slave)
os.close(slave)
output = b""
sent = False
deadline = time.monotonic() + 45
while time.monotonic() < deadline:
    ready, _, _ = select.select([master], [], [], 0.1)
    if ready:
        try:
            chunk = os.read(master, 65536)
        except OSError as error:
            if error.errno == errno.EIO:
                break
            raise
        if not chunk:
            break
        output += chunk
    if not sent and b"VM ready:" in output:
        os.write(master, b'printf "NAMU_GUEST=%s\n" "$(uname -s)"; exit\n')
        sent = True
    if proc.poll() is not None and not ready:
        break
else:
    proc.kill()
    raise RuntimeError("guest shell timed out: " + output.decode(errors="replace"))
stdout = proc.communicate(timeout=5)[0]
os.close(master)
print(json.dumps({"Output": output.decode(errors="replace"), "Stdout": stdout.decode(), "Code": proc.returncode}))
`
