package guest

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/kamaal111/namu/internal/vm/credentials"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
)

//go:embed profile.py
var profileScript string

//go:embed remove-profile.py
var removeProfileScript string

//go:embed git-credential-host.sh
var gitCredentialHelper string

type profilePayload struct {
	credentials.Bundle
	GitHelper string `json:"git_helper"`
}

func (s SSH) ImportCredentials(ctx context.Context, bundle credentials.Bundle) (string, error) {
	if _, err := registry.Resolve(s.VM.OS); err != nil {
		return "", err
	}
	payload, err := json.Marshal(profilePayload{Bundle: bundle, GitHelper: gitCredentialHelper})
	if err != nil {
		return "", fmt.Errorf("encode host credentials; contents withheld")
	}
	output, err := s.Run(ctx, host.Join([]string{"python3", "-c", profileScript}), bytes.NewReader(payload), nil, nil, false)
	if err != nil {
		return "", fmt.Errorf("prepare private guest credential profile; credential contents withheld: %w", err)
	}
	var profile string
	if json.Unmarshal(output, &profile) != nil || !validProfile(profile, s.VM.OS) {
		return "", fmt.Errorf("invalid guest credential profile response; contents withheld")
	}
	return profile, nil
}
func validProfile(profile, osName string) bool {
	definition, err := registry.Resolve(osName)
	if err != nil {
		return false
	}
	parent := definition.Home + "/.namu-vm/credentials"
	return path.Dir(profile) == parent && strings.HasPrefix(path.Base(profile), "session.") && !strings.ContainsAny(profile, "\x00\r\n")
}
func (s SSH) RemoveCredentials(ctx context.Context, profile string) error {
	if !validProfile(profile, s.VM.OS) {
		return fmt.Errorf("refusing to remove invalid guest credential profile")
	}
	_, err := s.Run(ctx, host.Join([]string{"python3", "-c", removeProfileScript, profile}), nil, io.Discard, io.Discard, false)
	return err
}
