// Package macos owns the supported macOS 27 guest implementation.
package macos

import (
	_ "embed"
	"github.com/kamaal111/namu/internal/vm/platform"
	"github.com/kamaal111/namu/internal/vm/resources"
)

const user = "admin"

//go:embed access.sh
var accessPreparation string

//go:embed essentials.sh
var essentials string

//go:embed agent-prerequisites.sh
var agentPrerequisites string

//go:embed mount.sh
var mountImplementation string

//go:embed mount-check.sh
var mountCheck string

func Definition() platform.Definition {
	return platform.Definition{
		Name: "macos", Label: "macOS 27", DefaultVMName: "namu-macos",
		User: user, Home: "/Users/admin", Kernel: "Darwin", DownloadPlatform: "darwin",
		Resources: resources.Settings{CPUs: 4, MemoryMB: 8192, DiskGB: 80},
		Host:      platform.HostRequirements{MinimumMacOSMajor: 27, Guidance: "use --os linux on an older supported host"},
		Hooks: platform.Hooks{
			Access:       func(publicKey string) string { return platform.Access(publicKey, user, accessPreparation) },
			Bootstrap:    func(publicKey string) string { return platform.Bootstrap(publicKey, user, accessPreparation, "", "") },
			Mount:        func(root, tag string) string { return platform.Mount(root, tag, mountImplementation) },
			MountPrelude: mountCheck, Essentials: essentials, AgentPrerequisites: agentPrerequisites,
		},
	}
}
