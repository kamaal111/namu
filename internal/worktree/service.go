// Package worktree coordinates Git mutations and post-creation setup.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamaal111/namu/internal/config"
	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/setup"
)

type Service struct {
	Repo     git.Repo
	Out, Err io.Writer
}

func (s *Service) path(ctx context.Context, branch string) (string, error) {
	if err := git.ValidateBranch(ctx, s.Repo.Source, branch); err != nil {
		return "", err
	}
	path := s.Repo.Main
	for _, part := range append([]string{".agents", "worktrees"}, strings.Split(branch, "/")...) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("managed path must contain only directories without symlinks: %s", path)
		}
	}
	return path, nil
}
func (s *Service) printPath(path string) error { _, err := fmt.Fprintln(s.Out, path); return err }
func (s *Service) ensureExcluded() error {
	root, err := os.OpenRoot(s.Repo.Common)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.MkdirAll("info", 0700); err != nil {
		return err
	}
	b, err := root.ReadFile("info/exclude")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "/.agents/worktrees/" {
			return nil
		}
	}
	f, err := root.OpenFile("info/exclude", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintln(f, "\n# namu managed checkouts\n/.agents/worktrees/")
	return errors.Join(writeErr, f.Close())
}
func (s *Service) afterCreate(ctx context.Context, branch, path string, cfg config.Config) error {
	if err := s.printPath(path); err != nil {
		return fmt.Errorf("checkout preserved at %s; cannot print path: %w", path, err)
	}
	if err := setup.Run(ctx, s.Repo, path, cfg, s.Err); err != nil {
		return fmt.Errorf("setup failed; checkout preserved at %s; retry from %s: namu setup %s: %w", path, s.Repo.Source, shellArgument(branch), err)
	}
	return nil
}
func (s *Service) prepare(ctx context.Context, branch string) (string, config.Config, error) {
	var cfg config.Config
	path, err := s.path(ctx, branch)
	if err != nil {
		return "", cfg, err
	}
	if _, err = os.Lstat(path); err == nil {
		return "", cfg, fmt.Errorf("destination already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", cfg, err
	}
	cfg, err = config.Load(s.Repo.Source)
	if err != nil {
		return "", cfg, err
	}
	return path, cfg, nil
}
func (s *Service) createAt(ctx context.Context, branch, path string, args []string, cfg config.Config) error {
	if _, err := s.path(ctx, branch); err != nil {
		return err
	}
	if err := s.ensureExcluded(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if _, err := git.Run(ctx, s.Repo.Source, append([]string{"worktree", "add"}, args...)...); err != nil {
		return err
	}
	return s.afterCreate(ctx, branch, path, cfg)
}
func (s *Service) Create(ctx context.Context, branch, base, remote string) error {
	if _, err := git.Branches(ctx, s.Repo.Source); err != nil {
		return err
	}
	if err := git.ValidateBranch(ctx, s.Repo.Source, branch); err != nil {
		return err
	}
	if exists, err := s.localExists(ctx, branch); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("branch %s already exists; use namu open %s", branch, branch)
	}
	path, cfg, err := s.prepare(ctx, branch)
	if err != nil {
		return err
	}
	if base == "" {
		base, err = git.DefaultBase(ctx, s.Repo.Source, remote)
		if err != nil {
			return err
		}
	}
	revision, err := git.Text(ctx, s.Repo.Source, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return fmt.Errorf("cannot resolve base %q: %w", base, err)
	}
	return s.createAt(ctx, branch, path, []string{"-b", branch, "--", path, revision}, cfg)
}
func (s *Service) localExists(ctx context.Context, branch string) (bool, error) {
	text, err := git.Text(ctx, s.Repo.Source, "for-each-ref", "--format=%(refname)", "refs/heads/"+branch)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(text, "\n") {
		if line == "refs/heads/"+branch {
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) Open(ctx context.Context, label string) error {
	branches, err := git.AllBranches(ctx, s.Repo.Source)
	if err != nil {
		return err
	}
	var selected *git.Branch
	for _, b := range branches {
		if b.Remote == "" && (label == b.Name || label == b.Ref) {
			copy := b
			selected = &copy
			break
		}
	}
	if selected == nil {
		for _, b := range branches {
			if label == b.Label() || label == b.Ref || label == b.Name {
				if selected != nil {
					return fmt.Errorf("ambiguous branch %s; select a remote-qualified branch", label)
				}
				copy := b
				selected = &copy
			}
		}
	}
	if selected == nil {
		return fmt.Errorf("unknown branch %q; fetch remotes or use namu open to refresh branches", label)
	}
	if err := git.ValidateBranch(ctx, s.Repo.Source, selected.Name); err != nil {
		return err
	}
	if selected.Remote != "" {
		for _, b := range branches {
			if b.Remote == "" && b.Name == selected.Name {
				copy := b
				selected = &copy
				break
			}
		}
	}
	trees, err := git.Worktrees(ctx, s.Repo.Source)
	if err != nil {
		return err
	}
	for _, w := range trees {
		if w.Branch == selected.Name {
			path, err := filepath.EvalSymlinks(w.Path)
			if err != nil {
				return fmt.Errorf("registered checkout unavailable: %w", err)
			}
			return s.printPath(path)
		}
	}
	path, cfg, err := s.prepare(ctx, selected.Name)
	if err != nil {
		return err
	}
	args := []string{"--", path, selected.Name}
	if selected.Remote != "" {
		args = []string{"--track", "-b", selected.Name, "--", path, selected.Ref}
	}
	return s.createAt(ctx, selected.Name, path, args, cfg)
}
func (s *Service) managed(ctx context.Context, branch string) (string, error) {
	path, err := s.path(ctx, branch)
	if err != nil {
		return "", err
	}
	trees, err := git.Worktrees(ctx, s.Repo.Source)
	if err != nil {
		return "", err
	}
	for _, w := range trees {
		if w.Path == path && w.Branch == branch {
			if w.Locked != "" {
				return "", fmt.Errorf("worktree is locked: %s", w.Locked)
			}
			if w.Prunable != "" {
				return "", fmt.Errorf("worktree is prunable: %s", w.Prunable)
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("no matching registered managed checkout for %s", branch)
}
func (s *Service) Delete(ctx context.Context, branch string) error {
	path, err := s.managed(ctx, branch)
	if err != nil {
		return err
	}
	if s.Repo.Source == path || strings.HasPrefix(s.Repo.Source, path+string(filepath.Separator)) {
		return fmt.Errorf("cannot delete the invoking checkout")
	}
	if _, err := git.Run(ctx, s.Repo.Source, "worktree", "remove", "--", path); err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.Err, "Removed %s (branch %s retained)\n", path, branch)
	return err
}
func (s *Service) Setup(ctx context.Context, branch string) error {
	path, err := s.managed(ctx, branch)
	if err != nil {
		return err
	}
	if path == s.Repo.Source {
		return fmt.Errorf("run setup from the original source checkout")
	}
	cfg, err := config.Load(s.Repo.Source)
	if err != nil {
		return err
	}
	return setup.Run(ctx, s.Repo, path, cfg, s.Err)
}

// shellArgument keeps the printed retry command safe to paste into a POSIX shell.
func shellArgument(value string) string {
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./-", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
