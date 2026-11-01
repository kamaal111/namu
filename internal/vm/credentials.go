package vm

import (
	"context"
	"fmt"
	"time"

	"github.com/kamaal111/namu/internal/vm/credentials"
	"github.com/kamaal111/namu/internal/vm/guest"
	"github.com/kamaal111/namu/internal/vm/state"
)

func (m *Manager) prepareCredentials(ctx context.Context, v state.VM) error {
	if m.NoReuseCredentials {
		return nil
	}
	if m.profile != "" {
		if m.profileVM.ID != v.ID {
			return fmt.Errorf("credential profile belongs to another VM")
		}
		return nil
	}
	source, err := credentials.Default(m.Runner, m.SourceDirectory)
	if err != nil {
		return err
	}
	bundle, err := source.Collect(ctx)
	if err != nil {
		return err
	}
	connection, err := m.connection(ctx, v)
	if err != nil {
		return err
	}
	profile, err := connection.ImportCredentials(ctx, bundle)
	if err != nil {
		return err
	}
	m.profile, m.profileVM = profile, v
	for _, agent := range []struct {
		name, login string
		available   bool
	}{
		{"Codex", "codex login", len(bundle.Codex) > 0 || bundle.Environment["OPENAI_API_KEY"] != ""},
		{"Claude", "claude auth login", len(bundle.Claude) > 0 || bundle.Environment["ANTHROPIC_API_KEY"] != "" || bundle.Environment["ANTHROPIC_AUTH_TOKEN"] != "" || bundle.Environment["CLAUDE_CODE_OAUTH_TOKEN"] != ""},
		{"Cursor", "NO_OPEN_BROWSER=1 agent login", bundle.Environment["CURSOR_API_KEY"] != "" || bundle.Environment["CURSOR_AUTH_TOKEN"] != ""},
	} {
		if !agent.available {
			m.progress("No new %s host login found; existing imported login is retained. For guest-only login use namu vm shell %s --no-reuse-credentials, then %s", agent.name, v.Name, agent.login)
		}
	}
	m.progress("Host credential reuse enabled; use --no-reuse-credentials for guest-only logins")
	return nil
}

// Close removes transient credential environments; native guest refresh state stays in its separate profile.
func (m *Manager) Close() error {
	if m.profile == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := m.connection(ctx, m.profileVM)
	if err != nil {
		return err
	}
	if err := connection.RemoveCredentials(ctx, m.profile); err != nil {
		return fmt.Errorf("cannot remove transient guest credential profile: %w", err)
	}
	m.profile = ""
	return nil
}

func (m *Manager) sessionConnection(ctx context.Context, v state.VM) (guest.SSH, error) {
	if err := m.prepareCredentials(ctx, v); err != nil {
		return guest.SSH{}, err
	}
	connection, err := m.connection(ctx, v)
	if err != nil {
		return guest.SSH{}, err
	}
	if !m.NoReuseCredentials {
		connection.CredentialsDir, connection.ForwardAgent = m.profile, true
	}
	return connection, nil
}
