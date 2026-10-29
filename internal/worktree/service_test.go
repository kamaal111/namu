package worktree

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/testutil"
)

func ok(t *testing.T, value bool, message string) {
	t.Helper()
	if !value {
		t.Fatal(message)
	}
}
func write(t *testing.T, path, content string) { t.Helper(); testutil.Write(t, path, content) }
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return testutil.Git(t, dir, args...)
}
func fixture(t *testing.T) (*Service, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	repo := testutil.Repository(t)
	out, diagnostics := new(bytes.Buffer), new(bytes.Buffer)
	return &Service{Repo: repo, Out: out, Err: diagnostics}, out, diagnostics
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func TestCreateExplicitBaseAndLinkedInvocation(t *testing.T) {
	s, out, _ := fixture(t)
	ctx := context.Background()
	must(t, s.Create(ctx, "feature/cards", "main", ""))
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	ok(t, out.String() == path+"\n", out.String())
	repo, err := git.Discover(ctx, filepath.Join(path, ".git", ".."))
	must(t, err)
	s.Repo = repo
	out.Reset()
	must(t, s.Create(ctx, "feature/pricing", "feature/cards", ""))
	ok(t, out.String() == filepath.Join(s.Repo.Main, ".agents/worktrees/feature/pricing")+"\n", out.String())
	ok(t, run(t, s.Repo.Main, "status", "--porcelain") == "", "managed checkouts must be ignored")
}
func TestCreateDefaultRemoteAndNonMainDefault(t *testing.T) {
	s, out, _ := fixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, s.Repo.Main, "clone", "--bare", s.Repo.Main, remote)
	run(t, remote, "branch", "-m", "main", "trunk")
	run(t, s.Repo.Main, "remote", "add", "origin", remote)
	must(t, s.Create(context.Background(), "feature/cards", "", ""))
	ok(t, strings.HasSuffix(out.String(), "feature/cards\n"), out.String())
	ok(t, run(t, s.Repo.Main, "rev-parse", "feature/cards") == run(t, remote, "rev-parse", "trunk"), "must use remote default")
}
func TestCreateRejectsDuplicateBranchAndDestination(t *testing.T) {
	s, out, _ := fixture(t)
	ctx := context.Background()
	ok(t, s.Create(ctx, "main", "main", "") != nil, "existing branch must fail")
	write(t, filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards/file"), "occupied")
	ok(t, s.Create(ctx, "feature/cards", "main", "") != nil, "occupied path must fail")
	ok(t, out.Len() == 0, "failures must not print path")
}
func TestSetupFailureKeepsCheckoutAndPathThenRetries(t *testing.T) {
	s, out, diagnostics := fixture(t)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo setup-output; exit 7']\n")
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil, "hook must fail")
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	ok(t, out.String() == path+"\n", out.String())
	ok(t, strings.Contains(diagnostics.String(), "setup-output"), diagnostics.String())
	ok(t, strings.Contains(err.Error(), "namu setup feature/cards"), err.Error())
	ok(t, run(t, path, "branch", "--show-current") == "feature/cards", "checkout must remain")
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'touch ready']\n")
	must(t, s.Setup(context.Background(), "feature/cards"))
	_, err = os.Stat(filepath.Join(path, "ready"))
	must(t, err)
}
func TestOpenExistingCheckoutDoesNotRunSetup(t *testing.T) {
	s, out, _ := fixture(t)
	external := filepath.Join(t.TempDir(), "outside")
	run(t, s.Repo.Main, "worktree", "add", "-b", "feature/cards", external)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "invalid toml!!!")
	must(t, s.Open(context.Background(), "feature/cards"))
	physical, err := filepath.EvalSymlinks(external)
	must(t, err)
	ok(t, out.String() == physical+"\n", out.String())
}
func TestOpenRemoteCreatesTrackingBranch(t *testing.T) {
	s, out, _ := fixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, s.Repo.Main, "clone", "--bare", s.Repo.Main, remote)
	run(t, remote, "branch", "feature/search")
	run(t, s.Repo.Main, "remote", "add", "origin", remote)
	run(t, s.Repo.Main, "fetch", "origin")
	must(t, s.Open(context.Background(), "origin/feature/search"))
	ok(t, strings.HasSuffix(out.String(), "feature/search\n"), out.String())
	ok(t, run(t, s.Repo.Main, "config", "branch.feature/search.remote") == "origin", "must track remote")
}
func TestDeleteRetainsBranchAndRefusesDirtyOrCurrent(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	must(t, s.Create(ctx, "feature/cards", "main", ""))
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	write(t, filepath.Join(path, "untracked"), "keep")
	ok(t, s.Delete(ctx, "feature/cards") != nil, "untracked content must prevent deletion")
	must(t, os.Remove(filepath.Join(path, "untracked")))
	original := s.Repo
	linked, err := git.Discover(ctx, path)
	must(t, err)
	s.Repo = linked
	ok(t, s.Delete(ctx, "feature/cards") != nil, "cannot delete invoking checkout")
	s.Repo = original
	must(t, s.Delete(ctx, "feature/cards"))
	ok(t, run(t, s.Repo.Main, "branch", "--list", "feature/cards") != "", "branch must remain")
	_, err = os.Stat(path)
	ok(t, os.IsNotExist(err), "checkout must be removed")
}
func TestDeleteRejectsLockedExternalAndMain(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	must(t, s.Create(ctx, "feature/cards", "main", ""))
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	run(t, s.Repo.Main, "worktree", "lock", path)
	ok(t, s.Delete(ctx, "feature/cards") != nil, "locked checkout must remain")
	external := filepath.Join(t.TempDir(), "external")
	run(t, s.Repo.Main, "worktree", "add", "-b", "feature/external", external)
	ok(t, s.Delete(ctx, "feature/external") != nil, "external checkout must remain")
	ok(t, s.Delete(ctx, "main") != nil, "main checkout must remain")
}
func TestCreateRejectsSymlinkDestinationAndInvalidConfig(t *testing.T) {
	s, out, _ := fixture(t)
	must(t, os.MkdirAll(filepath.Join(s.Repo.Main, ".agents"), 0700))
	must(t, os.Symlink(t.TempDir(), filepath.Join(s.Repo.Main, ".agents/worktrees")))
	ok(t, s.Create(context.Background(), "feature/cards", "main", "") != nil, "symlink path must fail")
	ok(t, out.Len() == 0, "must fail before creation")
	must(t, os.Remove(filepath.Join(s.Repo.Main, ".agents/worktrees")))
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "invalid!!!")
	ok(t, s.Create(context.Background(), "feature/cards", "main", "") != nil, "invalid config must fail")
	ok(t, run(t, s.Repo.Main, "branch", "--list", "feature/cards") == "", "invalid config must not create branch")
}
func TestOpenUnknownBranchDoesNotMutate(t *testing.T) {
	s, out, _ := fixture(t)
	ok(t, s.Open(context.Background(), "unknown") != nil, "unknown branch must fail")
	ok(t, out.Len() == 0, "must not print a path")
	trees, err := git.Worktrees(context.Background(), s.Repo.Source)
	must(t, err)
	ok(t, len(trees) == 1, "must not create a checkout")
}

func TestOpenLocalBranchCreatesAttachedCheckout(t *testing.T) {
	s, out, _ := fixture(t)
	run(t, s.Repo.Source, "branch", "feature/cards")
	must(t, s.Open(context.Background(), "feature/cards"))
	path := strings.TrimSuffix(out.String(), "\n")
	ok(t, run(t, path, "branch", "--show-current") == "feature/cards", "must attach existing local branch")
}
func TestOpenQualifiedRemoteReusesLocalBranch(t *testing.T) {
	s, out, _ := fixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, s.Repo.Main, "clone", "--bare", s.Repo.Main, remote)
	run(t, remote, "branch", "feature/search")
	run(t, s.Repo.Main, "remote", "add", "origin", remote)
	run(t, s.Repo.Main, "fetch", "origin")
	must(t, s.Open(context.Background(), "origin/feature/search"))
	first := out.String()
	out.Reset()
	must(t, s.Open(context.Background(), "origin/feature/search"))
	ok(t, out.String() == first, "qualified remote must reuse local checkout")
}

func TestCreatePrintsPathBeforeSetupStarts(t *testing.T) {
	s, _, _ := fixture(t)
	outputPath := filepath.Join(t.TempDir(), "stdout")
	stream, err := os.Create(outputPath)
	must(t, err)
	t.Cleanup(func() { stream.Close() })
	s.Out = stream
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'test -s \"$1\"', 'setup', '"+outputPath+"']\n")
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	printed, err := os.ReadFile(outputPath)
	must(t, err)
	ok(t, string(printed) == filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")+"\n", "path must already exist when hook starts")
}
func TestDeleteRefusesTrackedChanges(t *testing.T) {
	s, _, _ := fixture(t)
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	write(t, filepath.Join(path, "tracked.txt"), "keep changes")
	ok(t, s.Delete(context.Background(), "feature/cards") != nil, "tracked changes must prevent deletion")
	data, err := os.ReadFile(filepath.Join(path, "tracked.txt"))
	must(t, err)
	ok(t, string(data) == "keep changes", "tracked changes must remain")
}
func TestCreateWithoutRemoteNeedsExplicitBase(t *testing.T) {
	s, out, _ := fixture(t)
	ok(t, s.Create(context.Background(), "feature/cards", "", "") != nil, "missing remote must fail")
	ok(t, out.Len() == 0, "missing remote must not print path")
	ok(t, run(t, s.Repo.Main, "branch", "--list", "feature/cards") == "", "missing remote must not create branch")
}
func TestOpenAmbiguousRemoteNameRequiresQualification(t *testing.T) {
	s, out, _ := fixture(t)
	head := run(t, s.Repo.Source, "rev-parse", "HEAD")
	run(t, s.Repo.Source, "update-ref", "refs/remotes/origin/feature/cards", head)
	run(t, s.Repo.Source, "update-ref", "refs/remotes/upstream/feature/cards", head)
	err := s.Open(context.Background(), "feature/cards")
	ok(t, err != nil && strings.Contains(err.Error(), "ambiguous"), "remote ambiguity must fail")
	ok(t, out.Len() == 0, "ambiguous branch must not print path")
}
func TestOpenLocalPrecedesRemoteAndPickerDeduplicates(t *testing.T) {
	s, out, _ := fixture(t)
	head := run(t, s.Repo.Source, "rev-parse", "HEAD")
	run(t, s.Repo.Source, "branch", "feature/cards")
	run(t, s.Repo.Source, "update-ref", "refs/remotes/origin/feature/cards", head)
	branches, err := git.Branches(context.Background(), s.Repo.Source)
	must(t, err)
	ok(t, len(branches) == 2, "picker should contain main and local feature only")
	must(t, s.Open(context.Background(), "feature/cards"))
	ok(t, run(t, strings.TrimSuffix(out.String(), "\n"), "branch", "--show-current") == "feature/cards", "must attach local branch")
}
func TestDuplicateCreationDoesNotCreateAnother(t *testing.T) {
	s, _, _ := fixture(t)
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil, "duplicate creation must fail")
	trees, err := git.Worktrees(context.Background(), s.Repo.Source)
	must(t, err)
	ok(t, len(trees) == 2, "only one linked checkout may exist")
}

func TestConcurrentCreationHasOneWinner(t *testing.T) {
	s, _, _ := fixture(t)
	var results [2]error
	var group sync.WaitGroup
	for i := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			other := Service{Repo: s.Repo, Out: io.Discard, Err: io.Discard}
			results[i] = other.Create(context.Background(), "feature/cards", "main", "")
		}()
	}
	group.Wait()
	ok(t, (results[0] == nil) != (results[1] == nil), "exactly one concurrent create must succeed")
	trees, err := git.Worktrees(context.Background(), s.Repo.Source)
	must(t, err)
	ok(t, len(trees) == 2, "concurrent create must retain one registered linked checkout")
	ok(t, run(t, filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards"), "branch", "--show-current") == "feature/cards", "winning checkout must retain its branch")
}

func TestRetryCommandQuotesShellCharacters(t *testing.T) {
	ok(t, shellArgument("feature/a'b;$HOME") == "'feature/a'\\''b;$HOME'", "retry branch must be quoted for shell use")
	ok(t, shellArgument("feature/cards") == "feature/cards", "ordinary branch should stay readable")
}
func TestRemoteNamesContainingSlash(t *testing.T) {
	s, out, _ := fixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, s.Repo.Main, "clone", "--bare", s.Repo.Main, remote)
	run(t, remote, "branch", "feature/search")
	run(t, s.Repo.Main, "remote", "add", "team/origin", remote)
	run(t, s.Repo.Main, "fetch", "team/origin")
	must(t, s.Open(context.Background(), "team/origin/feature/search"))
	ok(t, strings.HasSuffix(out.String(), "feature/search\n"), out.String())
	ok(t, run(t, s.Repo.Main, "config", "branch.feature/search.remote") == "team/origin", "remote name must be preserved")
}
