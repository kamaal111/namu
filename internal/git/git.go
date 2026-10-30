// Package git owns Git subprocesses and repository discovery.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Repo struct{ Source, Main, Common string }
type Worktree struct {
	Path, Branch, Head, Locked, Prunable string
	Bare                                 bool
}
type Branch struct{ Ref, Name, Remote string }

func (b Branch) Label() string {
	if b.Remote != "" {
		return b.Remote + "/" + b.Name
	}
	return b.Name
}

func Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return out.Bytes(), nil
}
func Text(ctx context.Context, dir string, args ...string) (string, error) {
	b, e := Run(ctx, dir, args...)
	return strings.TrimSuffix(string(b), "\n"), e
}
func Discover(ctx context.Context, dir string) (Repo, error) {
	bare, err := Text(ctx, dir, "rev-parse", "--is-bare-repository")
	if err != nil {
		return Repo{}, err
	}
	if bare == "true" {
		return Repo{}, fmt.Errorf("bare repositories are not supported")
	}
	source, err := Text(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return Repo{}, err
	}
	common, err := Text(ctx, source, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Repo{}, err
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return Repo{}, err
	}
	trees, err := Worktrees(ctx, source)
	if err != nil {
		return Repo{}, err
	}
	if len(trees) == 0 || trees[0].Bare {
		return Repo{}, fmt.Errorf("cannot locate main checkout")
	}
	main, err := filepath.EvalSymlinks(trees[0].Path)
	if err != nil {
		return Repo{}, fmt.Errorf("main checkout unavailable: %w", err)
	}
	return Repo{source, main, common}, nil
}
func Worktrees(ctx context.Context, dir string) ([]Worktree, error) {
	b, err := Run(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktrees(b), nil
}
func ParseWorktrees(b []byte) []Worktree {
	var result []Worktree
	var current *Worktree
	for _, field := range bytes.Split(b, []byte{0}) {
		s := string(field)
		key, value, _ := strings.Cut(s, " ")
		if key == "worktree" {
			result = append(result, Worktree{Path: value})
			current = &result[len(result)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "locked":
			current.Locked = value
			if value == "" {
				current.Locked = "locked"
			}
		case "prunable":
			current.Prunable = value
			if value == "" {
				current.Prunable = "prunable"
			}
		case "bare":
			current.Bare = true
		}
	}
	return result
}
func ValidateBranch(ctx context.Context, dir, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || name == "gitbutler/workspace" || strings.ContainsAny(name, "\x00\r\n") {
		return fmt.Errorf("invalid branch name: %q", name)
	}
	_, err := Run(ctx, dir, "check-ref-format", "refs/heads/"+name)
	return err
}
func AllBranches(ctx context.Context, dir string) ([]Branch, error) {
	if _, err := ResolveCommit(ctx, dir, "HEAD"); err != nil {
		return nil, fmt.Errorf("repository has no commits; create a commit first: %w", err)
	}
	text, err := Text(ctx, dir, "for-each-ref", "--format=%(refname)%09%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	remotes, err := Remotes(ctx, dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(remotes, func(i, j int) bool { return len(remotes[i]) > len(remotes[j]) })
	var all []Branch
	for _, line := range strings.Split(text, "\n") {
		ref, sym, _ := strings.Cut(line, "\t")
		if sym != "" {
			continue
		}
		b := Branch{Ref: ref}
		if n, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			b.Name = n
		} else if n, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
			b.Remote, b.Name, _ = strings.Cut(n, "/")
			for _, remote := range remotes {
				if name, ok := strings.CutPrefix(n, remote+"/"); ok {
					b.Remote = remote
					b.Name = name
					break
				}
			}
		} else {
			continue
		}
		if b.Name != "gitbutler/workspace" {
			all = append(all, b)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Label() < all[j].Label() })
	return all, nil
}

// Branches returns picker entries, preferring local branches over remote counterparts.
func Branches(ctx context.Context, dir string) ([]Branch, error) {
	all, err := AllBranches(ctx, dir)
	if err != nil {
		return nil, err
	}
	locals := make(map[string]bool)
	for _, b := range all {
		if b.Remote == "" {
			locals[b.Name] = true
		}
	}
	result := make([]Branch, 0, len(all))
	for _, b := range all {
		if b.Remote == "" || !locals[b.Name] {
			result = append(result, b)
		}
	}
	return result, nil
}

func Remotes(ctx context.Context, dir string) ([]string, error) {
	s, err := Text(ctx, dir, "remote")
	if err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}
func Fetch(ctx context.Context, dir, remote string) error {
	_, err := Run(ctx, dir, "fetch", "--no-tags", "--", remote)
	return err
}
func DefaultBase(ctx context.Context, dir, remote string) (string, error) {
	remotes, err := Remotes(ctx, dir)
	if err != nil {
		return "", err
	}
	chosen := ""
	for _, r := range remotes {
		if r == remote || (remote == "" && r == "origin") {
			chosen = r
		}
	}
	if remote == "" && chosen == "" && len(remotes) == 1 {
		chosen = remotes[0]
	}
	if chosen == "" {
		return "", fmt.Errorf("cannot select remote; specify --remote NAME or --base REF")
	}
	if err := Fetch(ctx, dir, chosen); err != nil {
		return "", err
	}
	s, err := Text(ctx, dir, "ls-remote", "--symref", "--", chosen, "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "ref: refs/heads/") {
			ref, tail, _ := strings.Cut(strings.TrimPrefix(line, "ref: refs/heads/"), "\t")
			if tail == "HEAD" {
				return "refs/remotes/" + chosen + "/" + ref, nil
			}
		}
	}
	return "", fmt.Errorf("remote %s has no resolvable default branch; supply --base REF", chosen)
}
