// Package github owns the optional GitHub CLI boundary.
package github

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"text/template"
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

const prBatchSize = 25

//go:embed pr_numbers.graphql
var prNumbersQuery string

func resolveNumbers(ctx context.Context, dir string, branches []string, numbers map[string]int) {
	unique := make([]string, 0, len(branches))
	seen := make(map[string]bool)
	for _, branch := range branches {
		if branch != "" && !seen[branch] {
			unique = append(unique, branch)
			seen[branch] = true
		}
	}
	if len(unique) == 0 {
		return
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return
	}
	output, err := run(ctx, dir, nil, "repo", "view", "--json", "nameWithOwner,url")
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
	if _, err := run(ctx, dir, nil, "auth", "status", "--active", "--hostname", u.Host); err != nil {
		return
	}
	owner, name, ok := strings.Cut(repo.NameWithOwner, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return
	}
	for start := 0; start < len(unique) && ctx.Err() == nil; start += prBatchSize {
		batch := unique[start:min(start+prBatchSize, len(unique))]
		for branch, number := range resolveBatch(ctx, dir, u.Host, owner, name, batch) {
			numbers[branch] = number
		}
	}
}

func resolveBatch(ctx context.Context, dir, host, owner, name string, branches []string) map[string]int {
	queryTemplate, err := template.New("pr_numbers").Parse(prNumbersQuery)
	if err != nil {
		return nil
	}
	var query bytes.Buffer
	if err := queryTemplate.Execute(&query, branches); err != nil {
		return nil
	}
	variables := map[string]string{"owner": owner, "name": name}
	for i, branch := range branches {
		variables[fmt.Sprintf("branch%d", i)] = branch
	}
	request := struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}{
		Query:     query.String(),
		Variables: variables,
	}
	input, err := json.Marshal(request)
	if err != nil {
		return nil
	}
	output, _ := run(ctx, dir, input, "api", "graphql", "--hostname", host, "--input", "-")
	var response struct {
		Data struct {
			Repository map[string]json.RawMessage `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Path []json.RawMessage `json:"path"`
		} `json:"errors"`
	}
	if json.Unmarshal(output, &response) != nil {
		return nil
	}
	for _, failure := range response.Errors {
		if len(failure.Path) < 2 {
			continue
		}
		var parent, alias string
		if json.Unmarshal(failure.Path[0], &parent) == nil && parent == "repository" && json.Unmarshal(failure.Path[1], &alias) == nil {
			delete(response.Data.Repository, alias)
		}
	}
	numbers := make(map[string]int)
	for i, branch := range branches {
		number, valid := connectionNumber(response.Data.Repository[fmt.Sprintf("open%d", i)])
		if !valid {
			continue
		}
		if number == 0 {
			number, _ = connectionNumber(response.Data.Repository[fmt.Sprintf("closed%d", i)])
		}
		if number > 0 {
			numbers[branch] = number
		}
	}
	return numbers
}

func connectionNumber(raw json.RawMessage) (int, bool) {
	var connection struct {
		Nodes *[]struct {
			Number int `json:"number"`
		} `json:"nodes"`
	}
	if json.Unmarshal(raw, &connection) != nil || connection.Nodes == nil {
		return 0, false
	}
	if len(*connection.Nodes) == 0 {
		return 0, true
	}
	number := (*connection.Nodes)[0].Number
	return number, number > 0
}

func run(ctx context.Context, dir string, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_SPINNER_DISABLED=1")
	cmd.WaitDelay = time.Second
	return cmd.Output()
}
