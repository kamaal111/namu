package vm

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
	"github.com/kamaal111/namu/internal/vm/credentials"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/kamaal111/namu/internal/worktree"
	"github.com/pelletier/go-toml/v2"
)

//go:embed testdata/live-shell.py
var liveShellProgram string

//go:embed testdata/live-setup.sh
var liveSetupScript string

//go:embed testdata/live-read-worktree.sh
var liveReadWorktreeScript string

//go:embed testdata/live-mounts.sh
var liveMountsScript string

//go:embed testdata/live-versions.sh
var liveVersionsScript string

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
	m.NoReuseCredentials = true
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
	configuration, err := toml.Marshal(map[string]map[string][]string{"setup": {"command": {"sh", "-c", liveSetupScript}}})
	require(t, err)
	testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), string(configuration))
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
	definition, err := registry.Resolve(saved.OS)
	require(t, err)
	expected := definition.Kernel
	assert(t, strings.TrimSpace(string(platform)) == expected, string(platform))
	assert(t, testutil.Git(t, path, "log", "-1", "--format=%s") == "guest", "guest Git commit must be visible on host")
	require(t, environment.Run(context.Background(), path, []string{"sh", "-c", liveReadWorktreeScript}, io.Discard))
	mounts, err := m.Tart.ExecRoot(context.Background(), name, liveMountsScript, nil)
	require(t, err)
	t.Log(string(mounts))
	connection, err := m.connection(context.Background(), environment.selected)
	require(t, err)
	profile, err := connection.ImportCredentials(context.Background(), credentials.Bundle{
		Codex:       json.RawMessage(`{"OPENAI_API_KEY":"synthetic-live-key"}`),
		Claude:      json.RawMessage(`{"claudeAiOauth":{"accessToken":"synthetic-live-token"}}`),
		Environment: map[string]string{"OPENAI_API_KEY": "synthetic-live-key", "CURSOR_AUTH_TOKEN": "synthetic-live-token", "GH_TOKEN": "synthetic-live-github"},
		GitName:     "Namu Smoke", GitEmail: "smoke@example.com",
	})
	require(t, err)
	t.Cleanup(func() { require(t, connection.RemoveCredentials(context.Background(), profile)) })
	connection.CredentialsDir = profile
	versions, err := connection.Run(context.Background(), liveVersionsScript, nil, nil, os.Stderr, false)
	require(t, err)
	assert(t, strings.Contains(string(versions), "codex-cli") && strings.Contains(string(versions), "Claude Code"), "all installed agents must start with a synthetic reused credential profile")
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
