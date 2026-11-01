package platform

import (
	_ "embed"
	"github.com/kamaal111/namu/internal/vm/host"
)

//go:embed root-shell.sh
var rootShellScript string

//go:embed access.sh
var accessScript string

//go:embed ssh-config.sh
var sshConfigScript string

//go:embed ssh-validate.sh
var sshValidateScript string

//go:embed ssh-public-key.sh
var sshPublicKeyScript string

//go:embed mount-point.sh
var mountPointScript string

func RootShell() string { return rootShellScript }

// Access restores only the approved key; host-key trust remains unchanged.
func Access(publicKey, user, preparation string) string {
	return rootShellScript + "user=" + host.Quote(user) + "\npublic_key=" + host.Quote(publicKey) + "\n" + preparation + accessScript
}

func Bootstrap(publicKey, user, preparation, beforeValidation, afterValidation string) string {
	return Access(publicKey, user, preparation) + sshConfigScript + beforeValidation + sshValidateScript + afterValidation + sshPublicKeyScript
}

func Mount(root, tag, implementation string) string {
	return "root=" + host.Quote(root) + "\ntag=" + host.Quote(tag) + "\n" + mountPointScript + implementation
}
