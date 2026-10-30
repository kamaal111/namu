package git_test

import (
	"context"
	"path/filepath"
	"testing"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/testutil"
)

func TestResolveCommitPeelsTagsAndRejectsNonCommitRefs(t *testing.T) {
	repo := testutil.Repository(t)
	ctx := context.Background()
	testutil.Git(t, repo.Source, "tag", "-am", "release", "v1")
	expected := testutil.Git(t, repo.Source, "rev-parse", "HEAD")
	actual, err := git.ResolveCommit(ctx, repo.Source, "v1")
	if err != nil || actual != expected {
		t.Fatalf("got %q, %v; want %q", actual, err, expected)
	}
	_, err = git.ResolveCommit(ctx, repo.Source, "HEAD:tracked.txt")
	if err == nil {
		t.Fatal("blob must not resolve as a commit")
	}
	_, err = git.ResolveCommit(ctx, repo.Source, "--help")
	if err == nil {
		t.Fatal("an option must not resolve as a commit")
	}
}

func TestLocalBranchExistsMatchesExactLocalRef(t *testing.T) {
	repo := testutil.Repository(t)
	ctx := context.Background()
	testutil.Git(t, repo.Source, "branch", "feature/cards")
	exists, err := git.LocalBranchExists(ctx, repo.Source, "feature")
	if err != nil || exists {
		t.Fatalf("prefix must not match: %v, %v", exists, err)
	}
	exists, err = git.LocalBranchExists(ctx, repo.Source, "feature/cards")
	if err != nil || !exists {
		t.Fatalf("expected local branch: %v, %v", exists, err)
	}
}

func TestAddWorktreeRefusesToReplaceCheckout(t *testing.T) {
	repo := testutil.Repository(t)
	request := git.WorktreeRequest{Path: filepath.Join(t.TempDir(), "checkout"), Branch: "feature/cards", Base: "main", CreateBranch: true}
	ctx := context.Background()
	if err := git.AddWorktree(ctx, repo.Source, request); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(request.Path, "keep"), "keep")
	err := git.AddWorktree(ctx, repo.Source, request)
	if err == nil {
		t.Fatal("must refuse to recreate an existing branch/checkout")
	}
	actual := testutil.Git(t, request.Path, "branch", "--show-current")
	if actual != request.Branch {
		t.Fatalf("branch changed to %q", actual)
	}
}
