package git

import (
	"context"
	"strings"
)

// ResolveCommit accepts a Git ref or object name and returns its commit ID.
func ResolveCommit(ctx context.Context, dir, ref string) (string, error) {
	return Text(ctx, dir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
}

// LocalBranchExists checks the exact local ref, excluding similarly named branches.
func LocalBranchExists(ctx context.Context, dir, branch string) (bool, error) {
	ref := "refs/heads/" + branch
	text, err := Text(ctx, dir, "for-each-ref", "--format=%(refname)", ref)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(text, "\n") {
		if line == ref {
			return true, nil
		}
	}
	return false, nil
}

// WorktreeRequest describes either a new branch or an existing local checkout.
// Base and Track apply when CreateBranch is true.
type WorktreeRequest struct {
	Path, Branch, Base  string
	CreateBranch, Track bool
}

// AddWorktree preserves Git's refusal to overwrite destinations or checked-out branches.
func AddWorktree(ctx context.Context, dir string, request WorktreeRequest) error {
	args := []string{"worktree", "add"}
	if request.CreateBranch {
		if request.Track {
			args = append(args, "--track")
		}
		args = append(args, "-b", request.Branch, "--", request.Path, request.Base)
	} else {
		args = append(args, "--", request.Path, request.Branch)
	}
	_, err := Run(ctx, dir, args...)
	return err
}
