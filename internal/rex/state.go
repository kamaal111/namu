package rex

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/integration"
)

type entry struct {
	Version   int    `json:"version"`
	Path      string `json:"path"`
	HostID    string `json:"host_id"`
	Branch    string `json:"branch"`
	SessionID string `json:"session_id"`
}

func session(ctx context.Context, request integration.OpenRequest, hostID string) (string, error) {
	path, err := filepath.EvalSymlinks(request.Path)
	if err != nil {
		return "", err
	}
	common, err := git.Text(ctx, request.Source, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	dir := filepath.Join(common, "namu", "rex")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(hostID + "\x00" + path))
	filename := filepath.Join(dir, fmt.Sprintf("%x.json", key))
	lock, err := os.OpenFile(filename+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("cannot lock Rex session mapping %s (another operation or a stale lock may exist): %w", filename, err)
	}
	defer os.Remove(filename + ".lock")
	if err := lock.Close(); err != nil {
		return "", err
	}
	saved := entry{}
	data, err := os.ReadFile(filename)
	if err == nil {
		if err := json.Unmarshal(data, &saved); err != nil {
			return "", fmt.Errorf("invalid Rex session mapping %s: %w", filename, err)
		}
		if saved.Version != 1 || saved.Path != path || saved.HostID != hostID || saved.Branch != request.Branch || saved.SessionID == "" {
			return "", fmt.Errorf("invalid Rex session mapping %s", filename)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var listed struct {
		Sessions *[]struct {
			ID string `json:"session_id"`
		} `json:"sessions"`
	}
	if err := readJSON(ctx, &listed, "ls", "--json"); err != nil {
		return "", err
	}
	if listed.Sessions == nil {
		return "", fmt.Errorf("Rex session list response is missing sessions")
	}
	for _, existing := range *listed.Sessions {
		if existing.ID == "" {
			return "", fmt.Errorf("Rex session list contains an empty session_id")
		}
		if existing.ID == saved.SessionID {
			return saved.SessionID, nil
		}
	}
	var created struct {
		ID string `json:"session_id"`
	}
	if err := readJSON(ctx, &created, "new", request.Branch, "--cwd", path, "--json"); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("Rex creation response is missing session_id; inspect Rex sessions before retrying")
	}
	data, err = json.Marshal(entry{Version: 1, Path: path, HostID: hostID, Branch: request.Branch, SessionID: created.ID})
	if err != nil {
		return "", err
	}
	if err := saveEntry(dir, filename, data); err != nil {
		return "", fmt.Errorf("Rex session %s created but mapping could not be saved; inspect before retrying: %w", created.ID, err)
	}
	return created.ID, nil
}

func saveEntry(dir, filename string, data []byte) error {
	file, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), filename)
}
