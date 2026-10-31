package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/integration"
	"github.com/kamaal111/namu/internal/pathdisplay"
)

// creationOutcome keeps recovery separate from the adapter's completion claim.
type creationOutcome struct {
	OpenHandled bool
	Recovered   bool
}

// createIntegrated verifies state after every external creation attempt before
// accepting it or safely falling back to Git.
func (s *Service) createIntegrated(ctx context.Context, request git.WorktreeRequest, revision string) (creationOutcome, error) {
	branch, path := request.Branch, request.Path
	result, hookErr := s.Integration.Create(ctx, integration.CreateRequest{Source: s.Repo.Source, Worktree: request, Revision: revision, Focus: !s.NoFocus})
	if ctx.Err() != nil {
		return creationOutcome{}, ctx.Err()
	}
	if hookErr != nil {
		result = integration.CreateResult{}
		fmt.Fprintf(s.Err, "Warning: %s creation failed; falling back to Git: %v\n", s.Integration.Name, hookErr)
	}
	trees, err := git.Worktrees(ctx, s.Repo.Source)
	if err != nil {
		return creationOutcome{}, err
	}
	for _, tree := range trees {
		if tree.Path != path && tree.Branch != branch {
			continue
		}
		actual, pathErr := filepath.EvalSymlinks(tree.Path)
		if pathErr != nil || actual != path || tree.Branch != branch || tree.Head != revision || tree.Locked != "" || tree.Prunable != "" {
			return creationOutcome{}, fmt.Errorf("%s left conflicting checkout state for %s; inspect git worktree list; existing state preserved", s.Integration.Name, branch)
		}
		head, err := git.ResolveCommit(ctx, path, "HEAD")
		if err != nil || head != revision {
			return creationOutcome{}, fmt.Errorf("cannot verify %s checkout at %s; existing state preserved", s.Integration.Name, pathdisplay.Shorten(path))
		}
		ref, err := git.Text(ctx, path, "symbolic-ref", "--quiet", "HEAD")
		if err != nil || ref != "refs/heads/"+branch {
			return creationOutcome{}, fmt.Errorf("cannot verify checkout branch at %s; existing state preserved", pathdisplay.Shorten(path))
		}
		return creationOutcome{OpenHandled: result.OpenHandled, Recovered: hookErr != nil}, nil
	}
	if hookErr == nil {
		fmt.Fprintf(s.Err, "Warning: %s did not register the requested checkout; falling back to Git\n", s.Integration.Name)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return creationOutcome{}, fmt.Errorf("cannot safely fall back: destination unavailable or already exists: %s; existing state preserved", pathdisplay.Shorten(path))
	}
	exists, err := git.LocalBranchExists(ctx, s.Repo.Source, branch)
	if err != nil {
		return creationOutcome{}, err
	}
	if exists {
		head, err := git.ResolveCommit(ctx, s.Repo.Source, "refs/heads/"+branch)
		if err != nil || head != revision {
			return creationOutcome{}, fmt.Errorf("cannot safely reuse branch %s after integration attempt; existing state preserved", branch)
		}
		request = git.WorktreeRequest{Path: path, Branch: branch}
	}
	if _, err := s.path(ctx, branch); err != nil {
		return creationOutcome{}, err
	}
	return creationOutcome{Recovered: true}, git.AddWorktree(ctx, s.Repo.Source, request)
}

func (s *Service) openRequest(path, branch string) integration.OpenRequest {
	return integration.OpenRequest{Source: s.Repo.Source, Path: path, Branch: branch, Focus: !s.NoFocus}
}

func (s *Service) openIntegrated(ctx context.Context, request integration.OpenRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Integration.Open != nil {
		err := s.Integration.Open(ctx, request)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			fmt.Fprintf(s.Err, "Warning: %s open failed; using existing Git checkout: %v\n", s.Integration.Name, err)
		}
		if err == nil {
			return nil
		}
	}
	return s.fallback(ctx, request)
}

func (s *Service) fallback(ctx context.Context, request integration.OpenRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Fallback.Open == nil {
		return nil
	}
	return s.Fallback.Open(ctx, request)
}
