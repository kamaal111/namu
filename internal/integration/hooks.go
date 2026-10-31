// Package integration defines the supported technology-specific operations:
// optional checkout creation and opening. Namu owns verification and recovery.
package integration

import (
	"context"

	git "github.com/kamaal111/namu/internal/git"
)

// CreateRequest describes the checkout Namu has selected for creation.
// Revision is the resolved commit that the resulting checkout must use.
type CreateRequest struct {
	Source   string
	Worktree git.WorktreeRequest
	Revision string
	Focus    bool
}

// OpenRequest identifies an existing checkout. Focus false means open without
// switching the user's current workspace or shell directory.
type OpenRequest struct {
	Source, Path, Branch string
	Focus                bool
}

// CreateResult describes work completed in addition to checkout creation.
// OpenHandled means opening with the requested focus behavior is complete.
// Its zero value asks Namu to call Open after verification and setup.
// Namu ignores this result if creation fails or requires Git recovery.
type CreateResult struct{ OpenHandled bool }

// CreateHook may create a checkout and optionally open it. Namu verifies actual
// Git state even when the hook returns an error or reports successful creation.
type CreateHook func(context.Context, CreateRequest) (CreateResult, error)

// OpenHook opens an existing checkout and honors its requested focus behavior.
// Return nil only when the request is complete; errors select shell fallback.
type OpenHook func(context.Context, OpenRequest) error

// Hooks lists every supported integration operation. A nil Create uses Git;
// a nil Open uses the navigation fallback when opening remains necessary.
type Hooks struct {
	Name   string
	Create CreateHook
	Open   OpenHook
}

// Shell requests navigation from the parent shell only after Namu completes.
func Shell(navigate func(string) error) Hooks {
	return Hooks{Name: "shell", Open: func(ctx context.Context, request OpenRequest) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !request.Focus || navigate == nil {
			return nil
		}
		return navigate(request.Path)
	}}
}
