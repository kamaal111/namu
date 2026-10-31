package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"

	"github.com/kamaal111/namu/internal/integration"
)

func Detected() bool { return os.Getenv("REX_SESSION") != "" }

func Hooks() integration.Hooks { return integration.Hooks{Name: "Rex", Open: open} }

func open(ctx context.Context, request integration.OpenRequest) error {
	return openWithTimeout(ctx, request, 60*time.Second)
}

func openWithTimeout(ctx context.Context, request integration.OpenRequest, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if utf8.RuneCountInString(request.Branch) > 128 {
		return fmt.Errorf("Rex session labels allow at most 128 characters; branch name is too long")
	}
	var identity struct {
		HostID    string `json:"host_id"`
		SessionID string `json:"session_id"`
		BlockID   string `json:"block_id"`
	}
	if err := readJSON(ctx, &identity, "whoami", "--json"); err != nil {
		return err
	}
	if identity.HostID == "" {
		return fmt.Errorf("Rex identity response is missing host_id")
	}
	sessionID, err := session(ctx, request, identity.HostID)
	if err != nil {
		return err
	}
	if !request.Focus {
		return nil
	}
	args := []string{}
	if os.Getenv("REX_CLIENT") == "" {
		owner, err := paneOwner(ctx, identity.SessionID, identity.BlockID)
		if err != nil {
			return err
		}
		if owner != "" {
			args = append(args, "-C", owner)
		}
	}
	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{sessionID})
	if err != nil {
		return err
	}
	_, err = run(ctx, append(args, "do", "session.select", "--args", string(payload))...)
	return err
}

// A missing or stale current pane leaves client selection to Rex's default.
func paneOwner(ctx context.Context, sessionID, blockID string) (string, error) {
	if sessionID == "" || blockID == "" {
		return "", nil
	}
	var info struct {
		Block struct {
			Owner string `json:"owner"`
		} `json:"block"`
	}
	if err := readJSON(ctx, &info, "block", "inspect", blockID, "--json"); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", nil
	}
	if info.Block.Owner == "" {
		return "", nil
	}
	var result struct {
		Clients []struct {
			ID    string `json:"client_id"`
			State string `json:"connection_state"`
			Info  struct {
				Kind string `json:"kind"`
			} `json:"info"`
		} `json:"clients"`
	}
	if err := readJSON(ctx, &result, "client", "ls", "--json"); err != nil {
		return "", err
	}
	for _, client := range result.Clients {
		if client.ID == info.Block.Owner && client.State == "connected" && client.Info.Kind == "app" {
			return client.ID, nil
		}
	}
	return "", nil
}
