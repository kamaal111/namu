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
	Herdr    bool
	NoFocus  bool
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
func (s *Service) createAt(ctx context.Context, request git.WorktreeRequest, revision, upstream string, cfg config.Config) error {
	branch, path := request.Branch, request.Path
	if _, err := s.path(ctx, branch); err != nil {
		return err
	}
	if err := s.ensureExcluded(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if s.Herdr {
		if err := s.createHerdr(ctx, request, revision); err != nil {
			return err
		}
		if upstream != "" {
			if _, err := git.Run(ctx, s.Repo.Source, "branch", "--set-upstream-to="+upstream, "--", branch); err != nil {
				return fmt.Errorf("checkout preserved at %s; cannot configure upstream: %w", path, err)
			}
		}
	} else if err := git.AddWorktree(ctx, s.Repo.Source, request); err != nil {
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
	if exists, err := git.LocalBranchExists(ctx, s.Repo.Source, branch); err != nil {
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
	revision, err := git.ResolveCommit(ctx, s.Repo.Source, base)
	if err != nil {
		return fmt.Errorf("cannot resolve base %q: %w", base, err)
	}
	return s.createAt(ctx, git.WorktreeRequest{Branch: branch, Path: path, Base: revision, CreateBranch: true}, revision, "", cfg)
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
			if s.Herdr {
				if err := s.herdr(ctx, "open", path, "", ""); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					fmt.Fprintf(s.Err, "Warning: Herdr open failed; using existing Git checkout: %v\n", err)
				}
			}
			return s.printPath(path)
		}
	}
	path, cfg, err := s.prepare(ctx, selected.Name)
	if err != nil {
		return err
	}
	request := git.WorktreeRequest{Branch: selected.Name, Path: path}
	if selected.Remote != "" {
		request.CreateBranch, request.Track, request.Base = true, true, selected.Ref
	}
	revision, err := git.ResolveCommit(ctx, s.Repo.Source, selected.Ref)
	if err != nil {
		return err
	}
	upstream := ""
	if selected.Remote != "" {
		upstream = selected.Ref
	}
	return s.createAt(ctx, request, revision, upstream, cfg)
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
	if err := git.ValidateBranch(ctx, s.Repo.Source, branch); err != nil {
		return err
	}
	trees, err := git.Worktrees(ctx, s.Repo.Source)
	if err != nil {
		return err
	}
	var selected *git.Worktree
	for _, tree := range trees {
		if tree.Branch == branch {
			if selected != nil {
				return fmt.Errorf("multiple registered checkouts for %s", branch)
			}
			selected = &tree
		}
	}
	if selected == nil {
		return fmt.Errorf("no matching registered checkout for %s", branch)
	}
	if selected.Locked != "" {
		return fmt.Errorf("worktree is locked: %s", selected.Locked)
	}
	if selected.Prunable != "" {
		return fmt.Errorf("worktree is prunable: %s", selected.Prunable)
	}
	path, err := filepath.EvalSymlinks(selected.Path)
	if err != nil {
		return fmt.Errorf("registered checkout unavailable: %w", err)
	}
	if path == s.Repo.Main {
		return fmt.Errorf("cannot delete the main checkout")
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
