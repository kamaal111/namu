package vm

import (
	"context"
	"fmt"
	"io"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"github.com/kamaal111/namu/internal/vm/state"
)

// Worktree binds a selected VM to namu's execution boundary, without owning Git lifecycle.
type Worktree struct {
	Manager  *Manager
	Name, OS string
	Default  bool
	ChooseOS func(context.Context) (string, error)
	selected state.VM
}

func (w *Worktree) Prepare(ctx context.Context, repo git.Repo, path string) error {
	w.Manager.SourceDirectory = repo.Source
	name, osName := w.Name, w.OS
	if w.Default && osName == "" {
		exists, err := w.Manager.Exists(registry.Default().DefaultVMName)
		if err != nil {
			return err
		}
		osName = registry.Default().Name
		if !exists && w.ChooseOS != nil {
			osName, err = w.ChooseOS(ctx)
			if err != nil {
				return err
			}
		}
	}
	if w.Default {
		definition, err := registry.Resolve(osName)
		if err != nil {
			return err
		}
		name = definition.DefaultVMName
	}
	roots := []string{repo.Main, repo.Common}
	if !contains(repo.Main, path) {
		roots = append(roots, path)
	}
	v, err := w.Manager.Ensure(ctx, name, osName, roots, w.Default)
	if err != nil {
		return err
	}
	w.selected = v
	return nil
}
func (w *Worktree) Run(ctx context.Context, path string, argv []string, out io.Writer) error {
	return w.Manager.Run(ctx, w.selected, path, argv, out)
}
func (w *Worktree) Open(ctx context.Context, path string, focus bool) error {
	if !focus {
		return w.Manager.Run(ctx, w.selected, path, []string{"true"}, w.Manager.Output)
	}
	return w.Manager.Shell(ctx, w.selected, path)
}
func (w *Worktree) RetryFlags() string {
	flags := fmt.Sprintf(" --vm --vm-name %s", host.Quote(w.selected.Name))
	if w.Manager.NoReuseCredentials {
		return flags + " --no-reuse-credentials"
	}
	return flags + " --reuse-credentials"
}

func (w *Worktree) Close() error { return w.Manager.Close() }
