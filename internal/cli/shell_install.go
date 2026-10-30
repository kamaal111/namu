package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

type shellEdit struct {
	Path     string
	Before   []byte
	Addition string
	Exists   bool
}

func prepareShellInstall(shell string) ([]shellEdit, error) {
	if shell != "bash" && shell != "zsh" {
		return nil, fmt.Errorf("unsupported shell %q; use bash or zsh", shell)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	paths := []string{filepath.Join(home, ".bashrc")}
	if shell == "zsh" {
		dir := os.Getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		paths = []string{filepath.Join(dir, ".zshrc")}
	} else {
		login := filepath.Join(home, ".bash_profile")
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			path := filepath.Join(home, name)
			_, err := os.Lstat(path)
			if err == nil {
				login = path
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		paths = append(paths, login)
	}
	eval := fmt.Sprintf("eval \"$(namu shell-init %s)\"", shell)
	guard := "command -v namu >/dev/null 2>&1"
	if shell == "bash" {
		guard = "[ -n \"${BASH_VERSION-}\" ] && " + guard
	}
	block := "# >>> namu shell integration >>>\nif " + guard + "; then\n  " + eval + "\nfi\n# <<< namu shell integration <<<\n"
	var edits []shellEdit
	seen := make(map[string]bool)
	for _, path := range paths {
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		_, err = os.Lstat(path)
		exists := err == nil
		if exists {
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		var before []byte
		if exists {
			info, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("startup file is not a regular file: %s", path)
			}
			before, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
		}
		addition := block
		for _, line := range strings.Split(string(before), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == eval || trimmed == fmt.Sprintf("eval \"`namu shell-init %s`\"", shell) {
				addition = ""
				break
			}
		}
		if addition != "" && len(before) > 0 && before[len(before)-1] != '\n' {
			addition = "\n" + addition
		}
		edits = append(edits, shellEdit{Path: path, Before: before, Addition: addition, Exists: exists})
	}
	return edits, nil
}

func previewShellInstall(edits []shellEdit, out io.Writer) error {
	for _, edit := range edits {
		if edit.Addition == "" {
			if _, err := fmt.Fprintf(out, "Already configured: %s\n", edit.Path); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(out, "Will add to: %s\n%s", edit.Path, edit.Addition); err != nil {
			return err
		}
	}
	return nil
}

func applyShellInstall(edits []shellEdit, out io.Writer) error {
	for _, edit := range edits {
		if edit.Addition == "" {
			if _, err := fmt.Fprintf(out, "Already configured: %s\n", edit.Path); err != nil {
				return err
			}
			continue
		}
		if err := appendShellEdit(edit, out); err != nil {
			return fmt.Errorf("cannot install shell integration in %s: %w", edit.Path, err)
		}
	}
	return nil
}

func appendShellEdit(edit shellEdit, out io.Writer) (result error) {
	if err := os.MkdirAll(filepath.Dir(edit.Path), 0700); err != nil {
		return err
	}
	flags := os.O_RDWR | os.O_APPEND | unix.O_NOFOLLOW
	if !edit.Exists {
		flags |= os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(edit.Path, flags, 0600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, f.Close()) }()
	current, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, edit.Before) {
		return fmt.Errorf("startup file changed since preview; retry installation")
	}
	n, writeErr := io.WriteString(f, edit.Addition)
	var reportErr error
	if n > 0 {
		label := "Added to"
		if n != len(edit.Addition) {
			label = "Partially added to"
		}
		_, reportErr = fmt.Fprintf(out, "%s: %s\n%s", label, edit.Path, edit.Addition[:n])
	}
	return errors.Join(writeErr, reportErr)
}

func reloadShellInstruction(shell string, out io.Writer) error {
	_, err := fmt.Fprintf(out, "Reload your shell with: exec %s\nThe current command cannot change its parent shell's directory; create/open will navigate after reloading.\n", shell)
	return err
}

func offerShellInstall(ctx context.Context, shell string, out io.Writer, confirm func() (bool, error)) error {
	edits, err := prepareShellInstall(shell)
	if err != nil {
		return err
	}
	if err := previewShellInstall(edits, out); err != nil {
		return err
	}
	needsInstall := false
	for _, edit := range edits {
		needsInstall = needsInstall || edit.Addition != ""
	}
	if needsInstall {
		accepted, err := confirm()
		if err != nil {
			return err
		}
		if !accepted {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := applyShellInstall(edits, out); err != nil {
			return err
		}
	}
	return reloadShellInstruction(shell, out)
}

func confirmShellInstall(ctx context.Context, input io.Reader, out io.Writer) (bool, error) {
	if _, err := fmt.Fprint(out, "Install shell integration with these changes? [y/N] "); err != nil {
		return false, err
	}
	var answer strings.Builder
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if file, ok := input.(*os.File); ok {
			fds := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLIN}}
			n, err := unix.Poll(fds, 100)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				return false, err
			}
			if n == 0 {
				continue
			}
		}
		var b [1]byte
		n, err := input.Read(b[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		if n == 0 {
			continue
		}
		if b[0] == '\n' {
			response := strings.TrimSpace(answer.String())
			return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes"), ctx.Err()
		}
		if answer.Len() >= 1024 {
			return false, nil
		}
		answer.WriteByte(b[0])
	}
}

func autoShellInstall(cmd *cobra.Command, _ []string) error {
	focus, _ := cmd.Flags().GetBool("focus")
	noFocus, _ := cmd.Flags().GetBool("no-focus")
	if !focus || noFocus || os.Getenv("NAMU_SHELL_INTEGRATION") == "1" || !interactive(cmd) || !terminalStream(cmd.OutOrStdout()) {
		return nil
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	if shell != "bash" && shell != "zsh" {
		return nil
	}
	err := offerShellInstall(cmd.Context(), shell, cmd.ErrOrStderr(), func() (bool, error) {
		return confirmShellInstall(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr())
	})
	if err != nil {
		if cmd.Context().Err() != nil {
			return cmd.Context().Err()
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: shell integration setup failed: %v\n", err)
	}
	return nil
}

func terminalStream(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(file.Fd()), terminalRequest())
	return err == nil
}
