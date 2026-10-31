package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/integration"
)

func TestOpenOnlyIntegrationRunsAfterGitAndSetup(t *testing.T) {
	s, out, _ := fixture(t)
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo ready > ready']\n")
	calls, fallbacks := 0, 0
	s.Integration = integration.Hooks{Name: "editor", Open: func(ctx context.Context, request integration.OpenRequest) error {
		calls++
		content, err := os.ReadFile(filepath.Join(request.Path, "ready"))
		must(t, err)
		ok(t, string(content) == "ready\n", string(content))
		ok(t, request.Source == s.Repo.Source && request.Path == path && request.Branch == "feature/cards" && request.Focus, "must receive selected checkout")
		ok(t, run(t, path, "branch", "--show-current") == "feature/cards", "Git must have created checkout")
		ok(t, out.String() == path+"\n", "path must already be printed")
		return nil
	}}
	s.Fallback = integration.Shell(func(string) error { fallbacks++; return nil })
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	ok(t, calls == 1 && fallbacks == 0, "open hook must handle navigation once")
}

func TestGenericCreationHookUsesSharedRecoveryAndNavigation(t *testing.T) {
	create := func(ctx context.Context, request integration.CreateRequest) error {
		return git.AddWorktree(ctx, request.Source, request.Worktree)
	}
	noCheckout := func(context.Context, integration.CreateRequest) error { return nil }
	for _, tc := range []struct {
		name              string
		create            func(context.Context, integration.CreateRequest) error
		fail, handled     bool
		warning, fallback bool
		opens             int
	}{
		{name: "success", create: create, handled: true},
		{name: "creation-needs-open", create: create, opens: 1},
		{name: "partial-success", create: create, fail: true, handled: true, warning: true, fallback: true},
		{name: "failure", create: noCheckout, fail: true, warning: true, fallback: true},
		{name: "false-success", create: noCheckout, handled: true, warning: true, fallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, out, diagnostics := fixture(t)
			path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
			creates, opens, fallbacks := 0, 0, 0
			s.Integration = integration.Hooks{Name: "other", Create: func(ctx context.Context, request integration.CreateRequest) (integration.CreateResult, error) {
				creates++
				err := tc.create(ctx, request)
				must(t, err)
				return integration.CreateResult{OpenHandled: tc.handled}, map[bool]error{true: errors.New("unavailable"), false: nil}[tc.fail]
			}, Open: func(context.Context, integration.OpenRequest) error {
				opens++
				return nil
			}}
			s.Fallback = integration.Shell(func(destination string) error {
				fallbacks++
				ok(t, destination == path, destination)
				return nil
			})
			must(t, s.Create(context.Background(), "feature/cards", "main", ""))
			ok(t, creates == 1 && opens == tc.opens, "must not repeat integration creation or open")
			ok(t, fallbacks == map[bool]int{true: 1, false: 0}[tc.fallback], "unexpected shell fallback")
			ok(t, strings.Contains(diagnostics.String(), "Warning: other") == tc.warning, diagnostics.String())
			ok(t, out.String() == path+"\n", out.String())
			ok(t, run(t, path, "branch", "--show-current") == "feature/cards", "checkout must exist")
		})
	}
}

func TestOpenHookFailureFallsBackWithoutSetup(t *testing.T) {
	s, out, diagnostics := fixture(t)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "invalid toml")
	calls := 0
	s.Integration = integration.Hooks{Name: "editor", Open: func(context.Context, integration.OpenRequest) error {
		return errors.New("cannot connect")
	}}
	s.Fallback = integration.Shell(func(path string) error { calls++; ok(t, path == s.Repo.Main, path); return nil })
	must(t, s.Open(context.Background(), "main"))
	ok(t, calls == 1, "failed hook must use shell fallback")
	ok(t, strings.Contains(diagnostics.String(), "Warning: editor open failed"), diagnostics.String())
	ok(t, out.String() == s.Repo.Main+"\n", out.String())
}

func TestSetupFailureSkipsOpenAndShellNavigation(t *testing.T) {
	s, out, _ := fixture(t)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'exit 7']\n")
	calls := 0
	s.Integration = integration.Hooks{Name: "editor", Create: func(ctx context.Context, request integration.CreateRequest) (integration.CreateResult, error) {
		return integration.CreateResult{}, git.AddWorktree(ctx, request.Source, request.Worktree)
	}, Open: func(context.Context, integration.OpenRequest) error {
		calls++
		return nil
	}}
	s.Fallback = integration.Shell(func(string) error { calls++; return nil })
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil, "setup must fail")
	ok(t, calls == 0, "setup failure must prevent navigation")
	ok(t, strings.HasSuffix(out.String(), "/feature/cards\n"), out.String())
}

func TestOpenHookCancellationSkipsFallback(t *testing.T) {
	s, out, diagnostics := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	s.Integration = integration.Hooks{Name: "editor", Open: func(context.Context, integration.OpenRequest) error {
		cancel()
		return ctx.Err()
	}}
	s.Fallback = integration.Shell(func(string) error { calls++; return nil })
	err := s.Open(ctx, "main")
	ok(t, errors.Is(err, context.Canceled), "must propagate cancellation")
	ok(t, calls == 0 && diagnostics.Len() == 0 && out.Len() == 0, "must not navigate or warn after cancellation")
}

func TestSeparateCreateAndOpenHooksRunAroundSetup(t *testing.T) {
	s, _, _ := fixture(t)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo ready > ready']\n")
	var events []string
	s.Integration = integration.Hooks{Name: "editor", Create: func(ctx context.Context, request integration.CreateRequest) (integration.CreateResult, error) {
		events = append(events, "create")
		return integration.CreateResult{}, git.AddWorktree(ctx, request.Source, request.Worktree)
	}, Open: func(ctx context.Context, request integration.OpenRequest) error {
		content, err := os.ReadFile(filepath.Join(request.Path, "ready"))
		must(t, err)
		ok(t, string(content) == "ready\n", string(content))
		events = append(events, "open")
		return nil
	}}
	s.Fallback = integration.Shell(func(string) error { events = append(events, "shell"); return nil })
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	ok(t, strings.Join(events, ",") == "create,open", strings.Join(events, ","))
}

func TestCreateOnlyIntegrationUsesShellAfterSetup(t *testing.T) {
	for _, focus := range []bool{true, false} {
		t.Run(map[bool]string{true: "focus", false: "no-focus"}[focus], func(t *testing.T) {
			s, _, _ := fixture(t)
			s.NoFocus = !focus
			path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
			write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo ready > ready']\n")
			s.Integration = integration.Hooks{Name: "creator", Create: func(ctx context.Context, request integration.CreateRequest) (integration.CreateResult, error) {
				ok(t, request.Focus == focus, "create must receive focus preference")
				return integration.CreateResult{}, git.AddWorktree(ctx, request.Source, request.Worktree)
			}}
			calls := 0
			s.Fallback = integration.Shell(func(destination string) error {
				calls++
				ok(t, destination == path, destination)
				content, err := os.ReadFile(filepath.Join(destination, "ready"))
				must(t, err)
				ok(t, string(content) == "ready\n", string(content))
				return nil
			})
			must(t, s.Create(context.Background(), "feature/cards", "main", ""))
			ok(t, calls == map[bool]int{true: 1, false: 0}[focus], "shell must honor focus after setup")
		})
	}
}
