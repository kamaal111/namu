package herdr

import (
	"context"
	"os"

	"github.com/kamaal111/namu/internal/integration"
)

func Detected() bool { return os.Getenv("HERDR_ENV") == "1" }

// Hooks translates Namu's lifecycle into Herdr commands. Git verification and
// recovery belong to Namu, even when Herdr partially completes a command.
func Hooks() integration.Hooks {
	return integration.Hooks{Name: "Herdr", Create: create, Open: open}
}

func create(ctx context.Context, request integration.CreateRequest) (integration.CreateResult, error) {
	args := append([]string{"worktree", "create"}, checkoutArgs(request.Source, request.Worktree.Path, request.Focus)...)
	args = append(args, "--branch", request.Worktree.Branch, "--base", request.Revision)
	if err := Run(ctx, args...); err != nil {
		return integration.CreateResult{}, err
	}
	return integration.CreateResult{OpenHandled: true}, nil
}

func open(ctx context.Context, request integration.OpenRequest) error {
	args := append([]string{"worktree", "open"}, checkoutArgs(request.Source, request.Path, request.Focus)...)
	return Run(ctx, args...)
}

func checkoutArgs(source, path string, focus bool) []string {
	focusArg := "--no-focus"
	if focus {
		focusArg = "--focus"
	}
	return []string{"--cwd", source, "--path", path, focusArg}
}
