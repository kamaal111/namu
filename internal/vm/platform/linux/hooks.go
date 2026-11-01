// Package linux owns the supported Linux (Ubuntu 24.04) guest implementation.
package linux

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

//go:embed ssh-prepare.sh
var sshPrepare string

//go:embed ssh-restart.sh
var sshRestart string

func Definition() platform.Definition {
	return platform.Definition{
		Name: "linux", Label: "Linux (Ubuntu 24.04)", DefaultVMName: "namu-linux",
		User: user, Home: "/home/admin", Kernel: "Linux", DownloadPlatform: "linux",
		Resources: resources.Settings{CPUs: 4, MemoryMB: 8192, DiskGB: 60},
		Host:      platform.HostRequirements{MinimumMacOSMajor: 14, Guidance: ""},
		Hooks: platform.Hooks{
			Access: func(publicKey string) string { return platform.Access(publicKey, user, accessPreparation) },
			Bootstrap: func(publicKey string) string {
				return platform.Bootstrap(publicKey, user, accessPreparation, sshPrepare, sshRestart)
			},
			Mount:        func(root, tag string) string { return platform.Mount(root, tag, mountImplementation) },
			MountPrelude: "", Essentials: essentials, AgentPrerequisites: agentPrerequisites,
		},
	}
}
