package worktree

import (
	"context"
	"io"

	"github.com/kamaal111/namu/internal/config"
	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/setup"
)

// Environment is an explicit execution target. Its errors never select local fallback.
type Environment interface {
	Prepare(context.Context, git.Repo, string) error
	Run(context.Context, string, []string, io.Writer) error
	Open(context.Context, string, bool) error
	RetryFlags() string
}

func (s *Service) prepareEnvironment(ctx context.Context, path string) error {
	if s.Environment == nil {
		return nil
	}
	return s.Environment.Prepare(ctx, s.Repo, path)
}
func (s *Service) runSetup(ctx context.Context, path string, cfg config.Config) error {
	if s.Environment == nil {
		return setup.Run(ctx, s.Repo, path, cfg, s.Err)
	}
	return setup.RunWith(ctx, s.Repo, path, cfg, s.Err, s.Environment.Run)
}
func (s *Service) openEnvironment(ctx context.Context, path string) error {
	return s.Environment.Open(ctx, path, !s.NoFocus)
}
