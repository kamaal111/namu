package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/config"
	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/testutil"
	"golang.org/x/sys/unix"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, value bool, message string) {
	t.Helper()
	if !value {
		t.Fatal(message)
	}
}
func write(t *testing.T, path, text string)        { t.Helper(); testutil.Write(t, path, text) }
func run(t *testing.T, dir string, args ...string) { t.Helper(); testutil.Git(t, dir, args...) }
func fixture(t *testing.T, rules string) (git.Repo, string) {
	t.Helper()
	repo := testutil.Repository(t)
	source := repo.Source
	write(t, filepath.Join(source, ".gitignore"), ".env*\nlocal/\nsecrets/\n*.env\nvendor/\n\\#local\n")
	write(t, filepath.Join(source, ".worktreeinclude"), rules)
	run(t, source, "add", ".")
	run(t, source, "commit", "-qm", "initial")
	destination := filepath.Join(t.TempDir(), "checkout")
	run(t, source, "worktree", "add", "-qb", "feature", destination)
	destination, err := filepath.EvalSymlinks(destination)
	must(t, err)
	return repo, destination
}
func contents(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	must(t, err)
	return string(b)
}
func absent(t *testing.T, path string) {
	t.Helper()
	_, err := os.Lstat(path)
	check(t, os.IsNotExist(err), "unexpected file: "+path)
}
func TestIncludeCopiesOnlyIgnoredRegularFilesWithNegationAndPermissions(t *testing.T) {
	repo, dst := fixture(t, ".env*\n!.env.production\nnotes.txt\ntracked.txt\n")
	write(t, filepath.Join(repo.Source, ".env.local"), "local")
	write(t, filepath.Join(repo.Source, ".env.production"), "production")
	write(t, filepath.Join(repo.Source, "notes.txt"), "untracked")
	write(t, filepath.Join(repo.Source, "tracked.txt"), "changed")
	must(t, os.Chmod(filepath.Join(repo.Source, ".env.local"), 0640))
	must(t, Copy(context.Background(), repo.Source, dst))
	check(t, contents(t, filepath.Join(dst, ".env.local")) == "local", "missing copied secret")
	info, err := os.Stat(filepath.Join(dst, ".env.local"))
	must(t, err)
	check(t, info.Mode().Perm() == 0640, "permissions must be preserved")
	absent(t, filepath.Join(dst, ".env.production"))
	absent(t, filepath.Join(dst, "notes.txt"))
	check(t, contents(t, filepath.Join(dst, "tracked.txt")) == "initial", "tracked file changed")
}
func TestIncludeDirectoryAnchorsRecursivePatternsAndUnicode(t *testing.T) {
	repo, dst := fixture(t, "/root.env\nsecrets/\n!secrets/private/\n**/config.json\n\\#local\nlocal/\n")
	write(t, filepath.Join(repo.Source, "root.env"), "root")
	write(t, filepath.Join(repo.Source, "nested/root.env"), "nested")
	write(t, filepath.Join(repo.Source, "secrets/local/key"), "key")
	write(t, filepath.Join(repo.Source, "secrets/private/key"), "private")
	write(t, filepath.Join(repo.Source, "vendor/deep/config.json"), "config")
	write(t, filepath.Join(repo.Source, "#local"), "escaped")
	write(t, filepath.Join(repo.Source, "local/space newline\n雪"), "unicode")
	must(t, Copy(context.Background(), repo.Source, dst))
	check(t, contents(t, filepath.Join(dst, "root.env")) == "root", "root anchor")
	absent(t, filepath.Join(dst, "nested/root.env"))
	check(t, contents(t, filepath.Join(dst, "secrets/local/key")) == "key", "directory selection")
	absent(t, filepath.Join(dst, "secrets/private/key"))
	check(t, contents(t, filepath.Join(dst, "vendor/deep/config.json")) == "config", "recursive pattern")
	check(t, contents(t, filepath.Join(dst, "#local")) == "escaped", "escaped pattern")
	check(t, contents(t, filepath.Join(dst, "local/space newline\n雪")) == "unicode", "filename split")
}
func TestIncludePreservesExistingDestinationAndDeletedTrackedFile(t *testing.T) {
	repo, dst := fixture(t, ".env*\n")
	write(t, filepath.Join(repo.Source, ".env"), "source")
	write(t, filepath.Join(repo.Source, ".env.local"), "local")
	write(t, filepath.Join(repo.Source, ".env.tracked"), "source tracked")
	write(t, filepath.Join(dst, ".env"), "destination")
	must(t, os.Symlink("missing", filepath.Join(dst, ".env.local")))
	write(t, filepath.Join(dst, ".env.tracked"), "tracked")
	run(t, dst, "add", "-f", ".env.tracked")
	must(t, os.Remove(filepath.Join(dst, ".env.tracked")))
	must(t, Copy(context.Background(), repo.Source, dst))
	check(t, contents(t, filepath.Join(dst, ".env")) == "destination", "existing file replaced")
	link, err := os.Readlink(filepath.Join(dst, ".env.local"))
	must(t, err)
	check(t, link == "missing", "symlink replaced")
	absent(t, filepath.Join(dst, ".env.tracked"))
}
func TestIncludeRejectsSourceSymlinkAndStillCopiesOtherFile(t *testing.T) {
	repo, dst := fixture(t, ".env*\n")
	must(t, os.Symlink("tracked.txt", filepath.Join(repo.Source, ".env")))
	write(t, filepath.Join(repo.Source, ".env.local"), "local")
	check(t, Copy(context.Background(), repo.Source, dst) != nil, "source symlink must report failure")
	absent(t, filepath.Join(dst, ".env"))
	check(t, contents(t, filepath.Join(dst, ".env.local")) == "local", "other file must still copy")
}
func TestIncludeRejectsDestinationParentSymlink(t *testing.T) {
	repo, dst := fixture(t, "local/\n")
	write(t, filepath.Join(repo.Source, "local/key"), "secret")
	outside := t.TempDir()
	must(t, os.Symlink(outside, filepath.Join(dst, "local")))
	check(t, Copy(context.Background(), repo.Source, dst) != nil, "symlink parent must fail")
	absent(t, filepath.Join(outside, "key"))
}
func TestIncludeSpecialFileDoesNotBlock(t *testing.T) {
	repo, dst := fixture(t, "local/\n")
	must(t, os.MkdirAll(filepath.Join(repo.Source, "local"), 0700))
	must(t, unix.Mkfifo(filepath.Join(repo.Source, "local/pipe"), 0600))
	must(t, Copy(context.Background(), repo.Source, dst))
	absent(t, filepath.Join(dst, "local/pipe"))
}
func TestIncludeSkipsNestedRepository(t *testing.T) {
	repo, dst := fixture(t, "local/\n")
	nested := filepath.Join(repo.Source, "local/nested")
	must(t, os.MkdirAll(nested, 0700))
	run(t, nested, "init", "-q")
	write(t, filepath.Join(nested, "secret"), "secret")
	must(t, Copy(context.Background(), repo.Source, dst))
	absent(t, filepath.Join(dst, "local/nested/secret"))
}
func TestIncludeMissingAndCommentOnlyRulesAreNoops(t *testing.T) {
	repo, dst := fixture(t, "# comment\n\n")
	write(t, filepath.Join(repo.Source, ".env"), "secret")
	must(t, Copy(context.Background(), repo.Source, dst))
	absent(t, filepath.Join(dst, ".env"))
	must(t, os.Remove(filepath.Join(repo.Source, ".worktreeinclude")))
	must(t, Copy(context.Background(), repo.Source, dst))
	absent(t, filepath.Join(dst, ".env"))
}
func TestSetupCopiesBeforeHookAndSendsOutputToDiagnostics(t *testing.T) {
	repo, dst := fixture(t, ".env\n")
	write(t, filepath.Join(repo.Source, ".env"), "secret")
	var cfg config.Config
	cfg.Setup.Command = []string{"sh", "-c", "cat .env; printf hook"}
	var output bytes.Buffer
	must(t, Run(context.Background(), repo, dst, cfg, &output))
	check(t, output.String() == "secrethook", output.String())
}
func TestSetupCopyFailureSkipsHook(t *testing.T) {
	repo, dst := fixture(t, ".env\n")
	must(t, os.Symlink("tracked.txt", filepath.Join(repo.Source, ".env")))
	var cfg config.Config
	cfg.Setup.Command = []string{"sh", "-c", "touch should-not-exist"}
	check(t, Run(context.Background(), repo, dst, cfg, &bytes.Buffer{}) != nil, "copy failure must fail setup")
	absent(t, filepath.Join(dst, "should-not-exist"))
}
func TestSetupRejectsAnotherRepository(t *testing.T) {
	repo, _ := fixture(t, "")
	other, dst := fixture(t, "")
	check(t, repo.Common != other.Common, "fixture isolation")
	check(t, Run(context.Background(), repo, dst, config.Config{}, &bytes.Buffer{}) != nil, "foreign destination must fail")
}
func TestIncludeRejectsSymlinkRules(t *testing.T) {
	repo, dst := fixture(t, "")
	must(t, os.Remove(filepath.Join(repo.Source, ".worktreeinclude")))
	must(t, os.Symlink("tracked.txt", filepath.Join(repo.Source, ".worktreeinclude")))
	err := Copy(context.Background(), repo.Source, dst)
	check(t, err != nil && strings.Contains(err.Error(), "regular file"), "symlink rules must fail")
}

func TestIncludeInvalidPatternFailsWithoutCopying(t *testing.T) {
	repo, dst := fixture(t, "[\n.env\n")
	write(t, filepath.Join(repo.Source, ".env"), "secret")
	err := Copy(context.Background(), repo.Source, dst)
	check(t, err != nil && strings.Contains(err.Error(), ".worktreeinclude line 1"), "invalid pattern must fail with location")
	absent(t, filepath.Join(dst, ".env"))
}
