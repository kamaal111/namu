// Package registry explicitly registers guest operating systems, like CLI integrations.
package registry

import (
	"fmt"
	"github.com/kamaal111/namu/internal/vm/platform"
	"github.com/kamaal111/namu/internal/vm/platform/linux"
	"github.com/kamaal111/namu/internal/vm/platform/macos"
	"strings"
)

func All() []platform.Definition {
	return []platform.Definition{macos.Definition(), linux.Definition()}
}

func Default() platform.Definition { return macos.Definition() }

func Names() []string {
	definitions := All()
	names := make([]string, len(definitions))
	for i, definition := range definitions {
		names[i] = definition.Name
	}
	return names
}

func Resolve(name string) (platform.Definition, error) {
	for _, definition := range All() {
		if definition.Name == name {
			if err := definition.Validate(); err != nil {
				return platform.Definition{}, err
			}
			return definition, nil
		}
	}
	return platform.Definition{}, fmt.Errorf("unsupported guest OS %q; use %s", name, strings.Join(Names(), " or "))
}
