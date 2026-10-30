package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/herdr"
)

func (s *Service) herdr(ctx context.Context, action, path, branch, revision string) error {
	args := []string{"worktree", action, "--cwd", s.Repo.Source, "--path", path}
	if action == "create" {
		args = append(args, "--branch", branch, "--base", revision)
	}
	focus := "--focus"
	if s.NoFocus {
		focus = "--no-focus"
	}
	return herdr.Run(ctx, append(args, focus)...)
}

// createHerdr reconciles Git state even after Herdr reports success: workspace
// creation can fail after the checkout or branch has already been created.
func (s *Service) createHerdr(ctx context.Context, request git.WorktreeRequest, revision string) error {
	branch, path := request.Branch, request.Path
	herdrErr := s.herdr(ctx, "create", path, branch, revision)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if herdrErr != nil {
		fmt.Fprintf(s.Err, "Warning: Herdr creation failed; falling back to Git: %v\n", herdrErr)
	}
	trees, err := git.Worktrees(ctx, s.Repo.Source)
	if err != nil {
		return err
	}
	for _, tree := range trees {
		if tree.Path != path && tree.Branch != branch {
			continue
		}
		actual, pathErr := filepath.EvalSymlinks(tree.Path)
		if pathErr != nil || actual != path || tree.Branch != branch || tree.Head != revision || tree.Locked != "" || tree.Prunable != "" {
			return fmt.Errorf("Herdr left conflicting checkout state for %s; inspect git worktree list; existing state preserved", branch)
		}
		head, err := git.ResolveCommit(ctx, path, "HEAD")
		if err != nil || head != revision {
			return fmt.Errorf("cannot verify Herdr checkout at %s; existing state preserved", path)
		}
		ref, err := git.Text(ctx, path, "symbolic-ref", "--quiet", "HEAD")
		if err != nil || ref != "refs/heads/"+branch {
			return fmt.Errorf("cannot verify checkout branch at %s; existing state preserved", path)
		}
		return nil
	}
	if herdrErr == nil {
		fmt.Fprintln(s.Err, "Warning: Herdr did not register the requested checkout; falling back to Git")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("cannot safely fall back: destination unavailable or already exists: %s; existing state preserved", path)
	}
	exists, err := git.LocalBranchExists(ctx, s.Repo.Source, branch)
	if err != nil {
		return err
	}
	if exists {
		head, err := git.ResolveCommit(ctx, s.Repo.Source, "refs/heads/"+branch)
		if err != nil || head != revision {
			return fmt.Errorf("cannot safely reuse branch %s after Herdr attempt; existing state preserved", branch)
		}
		request = git.WorktreeRequest{Path: path, Branch: branch}
	}
	if _, err := s.path(ctx, branch); err != nil {
		return err
	}
	return git.AddWorktree(ctx, s.Repo.Source, request)
}
