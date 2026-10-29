// Package setup copies opt-in ignored files and runs an explicit project hook.
package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/kamaal111/namu/internal/config"
	git "github.com/kamaal111/namu/internal/git"
	"golang.org/x/sys/unix"
)

func Run(ctx context.Context, repo git.Repo, destination string, cfg config.Config, output io.Writer) error {
	target, err := git.Discover(ctx, destination)
	if err != nil {
		return err
	}
	if target.Source == repo.Source || target.Common != repo.Common {
		return fmt.Errorf("setup destination must be another checkout of the same repository")
	}
	if err := Copy(ctx, repo.Source, target.Source); err != nil {
		return err
	}
	if len(cfg.Setup.Command) == 0 {
		return nil
	}
	command := exec.CommandContext(ctx, cfg.Setup.Command[0], cfg.Setup.Command[1:]...)
	command.Dir = target.Source
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("setup hook: %w", err)
	}
	return nil
}
func names(data []byte) map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Split(string(data), "\x00") {
		if name != "" {
			out[name] = true
		}
	}
	return out
}
func Copy(ctx context.Context, source, destination string) error {
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(".worktreeinclude")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf(".worktreeinclude must be a regular file without symlinks")
	}
	rules, err := root.ReadFile(".worktreeinclude")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(rules)) == "" {
		return nil
	}
	ignored, err := git.Run(ctx, source, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	matcher, err := includeMatcher(string(rules))
	if err != nil {
		return err
	}
	tracked, err := git.Run(ctx, destination, "ls-files", "--cached", "-z")
	if err != nil {
		return err
	}
	protected := names(tracked)
	src, err := unix.Open(source, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(src)
	dst, err := unix.Open(destination, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(dst)
	var failures []error
	for _, name := range strings.Split(string(ignored), "\x00") {
		if !safe(name) || !matcher.Match(strings.Split(name, "/"), false) || protected[name] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !safe(name) {
			continue
		}
		if err := copyFile(src, dst, name); err != nil {
			failures = append(failures, fmt.Errorf("copy %q: %w", name, err))
		}
	}
	return errors.Join(failures...)
}
func safe(name string) bool {
	if !filepath.IsLocal(name) {
		return false
	}
	for _, p := range strings.Split(name, "/") {
		if p == ".git" || p == "." || p == ".." || p == "" {
			return false
		}
	}
	return true
}

// directory walks relative to a held descriptor, never following symlinks.
func directory(root int, parts []string, create bool) (int, error) {
	fd, err := unix.Dup(root)
	if err != nil {
		return -1, err
	}
	for _, part := range parts {
		if create {
			err = unix.Mkdirat(fd, part, 0700)
			if err != nil && !errors.Is(err, unix.EEXIST) {
				unix.Close(fd)
				return -1, err
			}
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
		var metadata unix.Stat_t
		if err := unix.Fstatat(fd, ".git", &metadata, unix.AT_SYMLINK_NOFOLLOW); err == nil {
			unix.Close(fd)
			return -1, fmt.Errorf("nested repository excluded")
		} else if !errors.Is(err, unix.ENOENT) {
			unix.Close(fd)
			return -1, err
		}
	}
	return fd, nil
}
func copyFile(source, destination int, name string) error {
	parts := strings.Split(name, "/")
	base := parts[len(parts)-1]
	parents := parts[:len(parts)-1]
	src, err := directory(source, parents, false)
	if err != nil {
		return err
	}
	defer unix.Close(src)
	inputFD, err := unix.Openat(src, base, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	input := os.NewFile(uintptr(inputFD), name)
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("only regular files can be copied")
	}
	dst, err := directory(destination, parents, true)
	if err != nil {
		return err
	}
	defer unix.Close(dst)
	outputFD, err := unix.Openat(dst, base, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if errors.Is(err, unix.EEXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	output := os.NewFile(uintptr(outputFD), name)
	_, copyErr := io.Copy(output, input)
	modeErr := output.Chmod(info.Mode().Perm())
	closeErr := output.Close()
	err = errors.Join(copyErr, modeErr, closeErr)
	if err != nil {
		cleanupErr := unix.Unlinkat(dst, base, 0)
		return errors.Join(err, cleanupErr)
	}
	return nil
}

func includeMatcher(rules string) (gitignore.Matcher, error) {
	var patterns []gitignore.Pattern
	for lineNumber, line := range strings.Split(rules, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, component := range strings.Split(strings.TrimPrefix(line, "!"), "/") {
			if _, err := filepath.Match(component, ""); err != nil {
				return nil, fmt.Errorf(".worktreeinclude line %d: %w", lineNumber+1, err)
			}
		}
		patterns = append(patterns, gitignore.ParsePattern(line, nil))
	}
	return gitignore.NewMatcher(patterns), nil
}
