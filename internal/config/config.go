package config

import (
	"errors"
	"fmt"
	"github.com/kamaal111/namu/internal/pathdisplay"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Defaults struct {
		Focus *bool `toml:"focus"`
		PR    *bool `toml:"pr"`
	} `toml:"defaults"`
	Setup struct {
		Command []string `toml:"command"`
	} `toml:"setup"`
}

func Load(source string) (Config, error) {
	var c Config
	root, err := os.OpenRoot(source)
	if err != nil {
		return c, err
	}
	defer root.Close()
	info, err := root.Lstat(".namu.toml")
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if !info.Mode().IsRegular() {
		return c, fmt.Errorf("%s must be a regular file", pathdisplay.Shorten(filepath.Join(source, ".namu.toml")))
	}
	f, err := root.Open(".namu.toml")
	if err != nil {
		return c, err
	}
	defer f.Close()
	if err := toml.NewDecoder(f).DisallowUnknownFields().Decode(&c); err != nil {
		return c, fmt.Errorf("read .namu.toml: %w", err)
	}
	if len(c.Setup.Command) > 0 && strings.TrimSpace(c.Setup.Command[0]) == "" {
		return c, fmt.Errorf("setup.command must begin with an executable")
	}
	for _, arg := range c.Setup.Command {
		if strings.ContainsRune(arg, 0) {
			return c, fmt.Errorf("setup.command contains NUL")
		}
	}
	return c, nil
}
