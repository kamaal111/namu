package git

import (
	"strings"
	"testing"
)

func TestParseWorktreesPreservesPathsAndStates(t *testing.T) {
	trees := ParseWorktrees([]byte("worktree /projects/space newline\n雪\x00HEAD abc123\x00branch refs/heads/feature/cards\x00locked reason\x00\x00worktree /projects/detached\x00HEAD def456\x00detached\x00prunable gitdir missing\x00\x00"))
	equal(t, len(trees), 2)
	equal(t, trees[0].Path, "/projects/space newline\n雪")
	equal(t, trees[0].Branch, "feature/cards")
	equal(t, trees[0].Locked, "reason")
	equal(t, trees[1].Head, "def456")
	equal(t, trees[1].Branch, "")
	equal(t, trees[1].Prunable, "gitdir missing")
}
func TestParseWorktreesEmptyLockAndBare(t *testing.T) {
	trees := ParseWorktrees([]byte("worktree /repo\x00bare\x00\x00worktree /linked\x00locked\x00\x00"))
	equal(t, trees[0].Bare, true)
	equal(t, trees[1].Locked, "locked")
}
func equal[T comparable](t *testing.T, actual, expected T) {
	t.Helper()
	if actual != expected {
		t.Fatalf("got %v; want %v", actual, expected)
	}
}
func TestBranchLabel(t *testing.T) {
	equal(t, (Branch{Name: "feature/cards", Remote: "upstream"}).Label(), "upstream/feature/cards")
	equal(t, strings.HasPrefix((Branch{Name: "main"}).Label(), "main"), true)
}
