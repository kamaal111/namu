// Package platform defines guest-OS metadata and script hooks; it owns no lifecycle or transport.
package platform

import (
	"fmt"
	"github.com/kamaal111/namu/internal/vm/resources"
)

type HostRequirements struct {
	MinimumMacOSMajor int
	Guidance          string
}

type Hooks struct {
	Access             func(publicKey string) string
	Bootstrap          func(publicKey string) string
	Mount              func(root, tag string) string
	MountPrelude       string
	Essentials         string
	AgentPrerequisites string
}

// Definition is a value returned by an adapter factory. Hooks only produce scripts;
// lifecycle, confirmation, and command execution remain with the caller.
type Definition struct {
	Name, Label, DefaultVMName           string
	User, Home, Kernel, DownloadPlatform string
	Resources                            resources.Settings
	Host                                 HostRequirements
	Hooks                                Hooks
}

func (d Definition) Validate() error {
	if d.Name == "" || d.Label == "" || d.DefaultVMName == "" || d.User == "" || d.Home == "" || d.Kernel == "" || d.DownloadPlatform == "" || d.Host.MinimumMacOSMajor < 1 || d.Hooks.Access == nil || d.Hooks.Bootstrap == nil || d.Hooks.Mount == nil || d.Hooks.Essentials == "" || d.Hooks.AgentPrerequisites == "" {
		return fmt.Errorf("incomplete guest OS definition %q", d.Name)
	}
	if err := d.Resources.Validate(); err != nil {
		return fmt.Errorf("invalid defaults for guest OS %s: %w", d.Name, err)
	}
	return nil
}
