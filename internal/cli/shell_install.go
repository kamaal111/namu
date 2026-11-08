package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamaal111/namu/internal/pathdisplay"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

type shellEdit struct {
	Path     string
	Before   []byte
	Addition string
	Exists   bool
	Replace  bool
}

func prepareShellInstall(shell string) ([]shellEdit, error) {
	if !supportedShell(shell) {
		return nil, fmt.Errorf("unsupported shell %q; use bash, zsh, or nu", shell)
	}
	if shell == "nu" {
		return prepareNuShellInstall()
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
		edit, err := readShellEdit(path)
		if err != nil {
			return nil, err
		}
		path = edit.Path
		if seen[path] {
			continue
		}
		seen[path] = true
		before := edit.Before
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
		edit.Addition = addition
		edits = append(edits, edit)
	}
	return edits, nil
}

func previewShellInstall(edits []shellEdit, out io.Writer) error {
	for _, edit := range edits {
		if edit.Addition == "" {
			if _, err := fmt.Fprintf(out, "Already configured: %s\n", pathdisplay.Shorten(edit.Path)); err != nil {
				return err
			}
		} else {
			label := "Will add to"
			if edit.Replace {
				label = "Will write"
			}
			if _, err := fmt.Fprintf(out, "%s: %s\n%s", label, pathdisplay.Shorten(edit.Path), edit.Addition); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyShellInstall(edits []shellEdit, out io.Writer) error {
	for _, edit := range edits {
		if edit.Addition == "" {
			if _, err := fmt.Fprintf(out, "Already configured: %s\n", pathdisplay.Shorten(edit.Path)); err != nil {
				return err
			}
			continue
		}
		if err := appendShellEdit(edit, out); err != nil {
			return fmt.Errorf("cannot install shell integration in %s: %w", pathdisplay.Shorten(edit.Path), err)
		}
	}
	return nil
}

func appendShellEdit(edit shellEdit, out io.Writer) (result error) {
	if err := os.MkdirAll(filepath.Dir(edit.Path), 0700); err != nil {
		return err
	}
	flags := os.O_RDWR | unix.O_NOFOLLOW
	if !edit.Replace {
		flags |= os.O_APPEND
	}
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
	if edit.Replace {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	n, writeErr := io.WriteString(f, edit.Addition)
	if edit.Replace && writeErr == nil {
		writeErr = f.Truncate(int64(n))
	}
	var reportErr error
	if n > 0 {
		label := "Added to"
		if edit.Replace {
			label = "Wrote"
		}
		if n != len(edit.Addition) {
			label = "Partially added to"
		}
		_, reportErr = fmt.Fprintf(out, "%s: %s\n%s", label, pathdisplay.Shorten(edit.Path), edit.Addition[:n])
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
	answer, err := readTerminalLine(ctx, input, out, "Install shell integration with these changes? [y/N] ")
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), err
}

func readTerminalLine(ctx context.Context, input io.Reader, out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	var answer strings.Builder
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if file, ok := input.(*os.File); ok {
			fds := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLIN}}
			n, err := unix.Poll(fds, 100)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				return "", err
			}
			if n == 0 {
				continue
			}
		}
		var b [1]byte
		n, err := input.Read(b[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", nil
			}
			return "", err
		}
		if n == 0 {
			continue
		}
		if b[0] == '\n' {
			response := strings.TrimSpace(answer.String())
			return response, ctx.Err()
		}
		if answer.Len() >= 1024 {
			return "", nil
		}
		answer.WriteByte(b[0])
	}
}

func autoShellInstall(cmd *cobra.Command, _ []string) error {
	if err := validateVMOptions(cmd); err != nil {
		return err
	}
	if vmEnabled(cmd) {
		_, err := selectedIntegration(cmd)
		return err
	}
	selected, err := selectedIntegration(cmd)
	if err != nil {
		return err
	}
	if selected.Name != "" {
		return nil
	}
	focus, _ := cmd.Flags().GetBool("focus")
	if !focus || os.Getenv("NAMU_SHELL_INTEGRATION") == "1" || !interactive(cmd) || !terminalStream(cmd.OutOrStdout()) {
		return nil
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	if !supportedShell(shell) {
		return nil
	}
	err = offerShellInstall(cmd.Context(), shell, cmd.ErrOrStderr(), func() (bool, error) {
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

func readShellEdit(path string) (shellEdit, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return shellEdit{}, err
	}
	_, err = os.Lstat(path)
	exists := err == nil
	if exists {
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return shellEdit{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return shellEdit{}, err
	}
	edit := shellEdit{Path: path, Exists: exists}
	if exists {
		info, err := os.Stat(path)
		if err != nil {
			return shellEdit{}, err
		}
		if !info.Mode().IsRegular() {
			return shellEdit{}, fmt.Errorf("startup file is not a regular file: %s", pathdisplay.Shorten(path))
		}
		edit.Before, err = os.ReadFile(path)
		if err != nil {
			return shellEdit{}, err
		}
	}
	return edit, nil
}

func prepareNuShellInstall() ([]shellEdit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "nu", "--no-config-file", "--commands", "$nu.user-autoload-dirs | first").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot locate Nushell autoload directory: %w", err)
	}
	dir := strings.TrimSuffix(string(output), "\n")
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("Nushell autoload directory must be absolute: %q", dir)
	}
	edit, err := readShellEdit(filepath.Join(dir, "namu.nu"))
	if err != nil {
		return nil, err
	}
	if edit.Exists && !bytes.HasPrefix(edit.Before, []byte("# Managed by namu shell-init nu --install.")) {
		return nil, fmt.Errorf("refusing to replace unmanaged Nushell integration: %s", pathdisplay.Shorten(edit.Path))
	}
	edit.Replace = true
	if string(edit.Before) != nuShellIntegration {
		edit.Addition = nuShellIntegration
	}
	return []shellEdit{edit}, nil
}
