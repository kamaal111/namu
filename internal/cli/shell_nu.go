package cli

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamaal111/namu/internal/pathdisplay"
	"github.com/spf13/cobra"
)

//go:embed scripts/shell-init.nu
var nuShellIntegration string

func nuShellDefinition() shellDefinition {
	return shellDefinition{name: "nu", writeInit: writeNuShellInit, prepareInstall: prepareNuShellInstall}
}

func writeNuShellInit(cmd *cobra.Command) error {
	_, err := io.WriteString(cmd.OutOrStdout(), nuShellIntegration)
	return err
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
