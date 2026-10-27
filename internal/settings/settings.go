// Package settings stores the small choices that are not the command list.
//
// The command list is a text file meant to be edited by hand. A terminal
// choice is a setting with one value per platform, so it lives in JSON next to
// the command list, and only the CLI writes it. The key is the platform
// because the same file is shared between Linux and macOS, and "Ghostty" is an
// application name on one and a program name on the other.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/h1st0ry3D/oshelly-cli/internal/config"
)

// Version is the shape of the settings file.
const Version = 1

// Settings is everything in the file.
type Settings struct {
	Version int `json:"version"`
	// Terminals maps a platform to the terminal to use there. An absent entry
	// means choose one automatically, which is the same as never having set it.
	Terminals map[string]string `json:"terminals,omitempty"`

	path string
}

// Default is the settings for a machine that has never chosen a terminal.
func Default() Settings {
	return Settings{Version: Version, Terminals: map[string]string{}}
}

// Path is where the settings live. They sit next to the command list, so moving
// OSHELLY_CONFIG moves both files together.
func Path() (string, error) {
	configPath, err := config.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "settings.json"), nil
}

// Load reads the file. A missing file is the defaults: a machine with no
// terminal choice lets the platform pick one.
func Load(path string) (Settings, error) {
	s := Default()
	s.path = path

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("%s will not parse: %w", path, err)
	}
	if err := s.normalize(); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Terminal is the choice for the platform this is running on, or empty when
// none has been made.
func (s Settings) Terminal() string { return s.Terminals[runtime.GOOS] }

// SetTerminal records a choice for this platform. An empty value clears it.
func (s *Settings) SetTerminal(value string) {
	value = strings.TrimSpace(value)
	if s.Terminals == nil {
		s.Terminals = map[string]string{}
	}
	if value == "" {
		delete(s.Terminals, runtime.GOOS)
	} else {
		s.Terminals[runtime.GOOS] = value
	}
	if len(s.Terminals) == 0 {
		s.Terminals = nil
	}
}

// Save writes the file, creating the directory, and replaces it atomically so
// an interrupted write cannot leave a half file.
func (s Settings) Save() error {
	if s.path == "" {
		return errors.New("settings have no path")
	}
	if err := s.normalize(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".oshelly-settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

// SetPath binds the file path so a loaded settings value can be saved back.
func (s *Settings) SetPath(path string) { s.path = path }

// normalize checks the platforms and drops empty values.
func (s *Settings) normalize() error {
	if s.Version != Version {
		return fmt.Errorf("version is %d, which is not %d", s.Version, Version)
	}
	if s.Terminals == nil {
		s.Terminals = map[string]string{}
	}
	for platform, value := range s.Terminals {
		if platform != "linux" && platform != "darwin" {
			return fmt.Errorf("%q is not a platform: use linux or darwin", platform)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			delete(s.Terminals, platform)
			continue
		}
		s.Terminals[platform] = value
	}
	if len(s.Terminals) == 0 {
		s.Terminals = nil
	}
	return nil
}
