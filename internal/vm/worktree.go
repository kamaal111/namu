package vm

import (
	"context"
	"fmt"
	"io"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/vm/host"
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
	name, osName := w.Name, w.OS
	if w.Default && osName == "" {
		exists, err := w.Manager.Exists("namu-macos")
		if err != nil {
			return err
		}
		osName = "macos"
		if !exists && w.ChooseOS != nil {
			osName, err = w.ChooseOS(ctx)
			if err != nil {
				return err
			}
		}
	}
	if w.Default {
		name = "namu-" + osName
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
	return fmt.Sprintf(" --vm --vm-name %s", host.Quote(w.selected.Name))
}
