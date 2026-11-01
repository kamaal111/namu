// Package agents owns guest tool installation and credential-aware launchers.
package agents

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform"
)

const Revision = "1"

//go:embed manifest.json
var manifest []byte

//go:embed install-environment.sh
var installEnvironment string

//go:embed install.sh
var installer string

//go:embed wrapper.sh
var wrapper string

//go:embed install-wrapper.sh
var installWrapperScript string

//go:embed finalize-install.sh
var installFinalizer string

type Manifest struct {
	CursorSHA map[string]string `json:"cursor_sha256"`
	Agents    map[string]string `json:"agents"`
}

func Script(definition platform.Definition) (string, error) {
	if err := definition.Validate(); err != nil {
		return "", err
	}
	var pins Manifest
	if err := json.Unmarshal(manifest, &pins); err != nil {
		return "", err
	}
	var script strings.Builder
	for _, pair := range [][2]string{{"revision", Revision}, {"platform", definition.DownloadPlatform}, {"cursor_sha", pins.CursorSHA[definition.Name]}, {"cursor_version", pins.Agents["cursor"]}} {
		if pair[1] == "" {
			return "", fmt.Errorf("missing agent bootstrap pin %s", pair[0])
		}
		fmt.Fprintf(&script, "%s=%s\n", pair[0], host.Quote(pair[1]))
	}
	script.WriteString(installEnvironment)
	script.WriteString(definition.Hooks.AgentPrerequisites)
	script.WriteString(installer)
	wrapperScript := strings.NewReplacer("@REVISION@", host.Quote(Revision), "@WRAPPER_BODY@", wrapper).Replace(installWrapperScript)
	for _, name := range []string{"codex", "claude", "agent", "cursor-agent"} {
		fmt.Fprintf(&script, "name=%s\n", host.Quote(name))
		script.WriteString(wrapperScript)
	}
	script.WriteString(installFinalizer)
	return script.String(), nil
}
