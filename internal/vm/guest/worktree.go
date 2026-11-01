package guest

import (
	"context"
	"fmt"
	"io"

	"github.com/kamaal111/namu/internal/vm/host"
)

func (s SSH) VerifyWorktree(ctx context.Context, path string, out io.Writer) error {
	script := "set -eu; cd " + host.Quote(path) + "; test -w .; git -c " + host.Quote("safe.directory="+path) + " rev-parse --is-inside-work-tree >/dev/null; git config --global --get-all safe.directory | grep -qxF -- " + host.Quote(path) + " || git config --global --add safe.directory " + host.Quote(path)
	_, err := s.Run(ctx, script, nil, out, out, false)
	if err != nil {
		return fmt.Errorf("guest cannot use Git worktree %s: %w", path, err)
	}
	return nil
}
func (s SSH) RunWorktree(ctx context.Context, path string, argv []string, out io.Writer) error {
	if err := s.VerifyWorktree(ctx, path, out); err != nil {
		return err
	}
	_, err := s.Run(ctx, "cd "+host.Quote(path)+" && exec "+host.Join(argv), nil, out, out, false)
	return err
}
func (s SSH) Shell(ctx context.Context, path string, in io.Reader, out io.Writer) error {
	prefix := ""
	if path != "" {
		if err := s.VerifyWorktree(ctx, path, out); err != nil {
			return err
		}
		prefix = "cd " + host.Quote(path) + " && "
	}
	_, err := s.Run(ctx, prefix+`exec "$SHELL" -l`, in, out, out, true)
	return err
}
