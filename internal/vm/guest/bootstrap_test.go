package guest

import (
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"os/exec"
	"strings"
	"testing"
)

func TestGeneratedGuestScriptsParseForBothOperatingSystems(t *testing.T) {
	for _, definition := range registry.All() {
		for _, script := range []string{AccessScript(definition, publicKey(1)), BootstrapScript(definition, publicKey(1)), MountScript(definition, []string{"/Users/ada/雪 project", "/Volumes/work/quote'project"})} {
			cmd := exec.Command("sh", "-n")
			cmd.Stdin = strings.NewReader(script)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatal(string(output), err)
			}
			if len(output) != 0 {
				t.Fatal(string(output))
			}
		}
	}
}
