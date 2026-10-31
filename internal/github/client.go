// Package github owns the optional GitHub CLI boundary.
package github

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// PRNumbers resolves branch PRs without making GitHub a requirement for listing.
func PRNumbers(ctx context.Context, dir string, branches []string) (map[string]int, error) {
	return prNumbers(ctx, dir, branches, 10*time.Second)
}

func prNumbers(ctx context.Context, dir string, branches []string, timeout time.Duration) (map[string]int, error) {
	lookup, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	numbers := make(map[string]int)
	resolveNumbers(lookup, dir, branches, numbers)
	return numbers, ctx.Err()
}

func resolveNumbers(ctx context.Context, dir string, branches []string, numbers map[string]int) {
	if _, err := exec.LookPath("gh"); err != nil {
		return
	}
	output, err := run(ctx, dir, "repo", "view", "--json", "nameWithOwner,url")
	if err != nil {
		return
	}
	var repo struct {
		NameWithOwner string `json:"nameWithOwner"`
		URL           string `json:"url"`
	}
	if json.Unmarshal(output, &repo) != nil {
		return
	}
	u, err := url.Parse(repo.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || repo.NameWithOwner == "" {
		return
	}
	if _, err := run(ctx, dir, "auth", "status", "--active", "--hostname", u.Host); err != nil {
		return
	}
	seen := make(map[string]bool)
	for _, branch := range branches {
		if ctx.Err() != nil {
			return
		}
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		for _, state := range []string{"open", "all"} {
			output, err := run(ctx, dir, "pr", "list", "--repo", u.Host+"/"+repo.NameWithOwner,
				"--head", branch, "--state", state, "--search", "sort:updated-desc", "--limit", "1", "--json", "number")
			if err != nil {
				break
			}
			var prs []struct {
				Number int `json:"number"`
			}
			if json.Unmarshal(output, &prs) != nil {
				break
			}
			if len(prs) == 0 {
				continue
			}
			if prs[0].Number > 0 {
				numbers[branch] = prs[0].Number
			}
			break
		}
	}
}

func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_SPINNER_DISABLED=1")
	cmd.WaitDelay = time.Second
	return cmd.Output()
}
