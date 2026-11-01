package worktree

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/integration"
)

type environmentFake struct {
	prepare func(context.Context, git.Repo, string) error
	run     func(context.Context, string, []string, io.Writer) error
	open    func(context.Context, string, bool) error
}

func (e environmentFake) Prepare(ctx context.Context, r git.Repo, p string) error {
	return e.prepare(ctx, r, p)
}
func (e environmentFake) Run(ctx context.Context, p string, a []string, o io.Writer) error {
	return e.run(ctx, p, a, o)
}
func (e environmentFake) Open(ctx context.Context, p string, f bool) error { return e.open(ctx, p, f) }
func (e environmentFake) RetryFlags() string                               { return " --vm --vm-name sandbox" }

func TestEnvironmentPreparationFailureDoesNotCreateOrFallback(t *testing.T) {
	s, out, _ := fixture(t)
	s.Environment = environmentFake{prepare: func(context.Context, git.Repo, string) error { return errors.New("VM declined") }}
	s.Fallback = integration.Shell(func(string) error { t.Fatal("must not fall back"); return nil })
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil && strings.Contains(err.Error(), "VM declined"), "must return preparation error")
	ok(t, out.Len() == 0, "must not print path")
	ok(t, run(t, s.Repo.Main, "branch", "--list", "feature/cards") == "", "must not create branch")
}
func TestEnvironmentCopiesBeforeGuestHookAndPrintsBeforeOpen(t *testing.T) {
	s, out, _ := fixture(t)
	write(t, filepath.Join(s.Repo.Source, ".gitignore"), ".env\n")
	write(t, filepath.Join(s.Repo.Source, ".worktreeinclude"), ".env\n")
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['guest-tool', 'argument with space']\n")
	run(t, s.Repo.Source, "add", ".")
	run(t, s.Repo.Source, "commit", "-qm", "setup")
	write(t, filepath.Join(s.Repo.Source, ".env"), "secret")
	events := []string{}
	s.Environment = environmentFake{
		prepare: func(_ context.Context, _ git.Repo, path string) error {
			_, err := os.Stat(path)
			ok(t, os.IsNotExist(err), "prepare must precede creation")
			events = append(events, "prepare")
			return nil
		},
		run: func(_ context.Context, path string, argv []string, _ io.Writer) error {
			b, err := os.ReadFile(filepath.Join(path, ".env"))
			must(t, err)
			ok(t, string(b) == "secret", "must copy before guest hook")
			ok(t, reflect.DeepEqual(argv, []string{"guest-tool", "argument with space"}), "must preserve argv")
			ok(t, out.String() == path+"\n", "must print before hook")
			events = append(events, "run")
			return nil
		},
		open: func(_ context.Context, path string, focus bool) error {
			ok(t, out.String() == path+"\n" && focus, "must print before guest shell")
			events = append(events, "open")
			return nil
		},
	}
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	ok(t, reflect.DeepEqual(events, []string{"prepare", "run", "open"}), "must preserve execution order")
}
func TestEnvironmentHookFailurePreservesCheckoutAndVMRetry(t *testing.T) {
	s, out, _ := fixture(t)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['guest-only']\n")
	s.Environment = environmentFake{
		prepare: func(context.Context, git.Repo, string) error { return nil },
		run:     func(context.Context, string, []string, io.Writer) error { return errors.New("guest failed") },
	}
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil && strings.Contains(err.Error(), "namu setup feature/cards --vm --vm-name sandbox"), "must include selected VM in retry")
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	ok(t, out.String() == path+"\n", "must retain printed path")
	ok(t, run(t, path, "branch", "--show-current") == "feature/cards", "must retain checkout")
}
func TestEnvironmentExistingOpenDoesNotRerunSetupOrNavigate(t *testing.T) {
	s, out, _ := fixture(t)
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	out.Reset()
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "invalid!")
	s.NoFocus = true
	events := []string{}
	s.Environment = environmentFake{
		prepare: func(context.Context, git.Repo, string) error { events = append(events, "prepare"); return nil },
		open: func(_ context.Context, path string, focus bool) error {
			ok(t, !focus && out.String() == path+"\n", "must prepare without focusing")
			events = append(events, "open")
			return nil
		},
	}
	s.Fallback = integration.Shell(func(string) error { t.Fatal("must not navigate"); return nil })
	must(t, s.Open(context.Background(), "feature/cards"))
	ok(t, reflect.DeepEqual(events, []string{"prepare", "open"}), "must not run setup")
}
