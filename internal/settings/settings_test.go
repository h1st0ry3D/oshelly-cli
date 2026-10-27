package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOfAMissingFileIsTheDefaults(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Terminal() != "" {
		t.Errorf("terminal = %q, want no choice", s.Terminal())
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	s.SetPath(path)
	s.SetTerminal("Ghostty")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Terminal() != "Ghostty" {
		t.Errorf("terminal = %q, want Ghostty", got.Terminal())
	}
}

// The file is shared across platforms, so a choice made on a Mac must not
// become a Linux program name.
func TestAChoiceIsStoredForThisPlatformOnly(t *testing.T) {
	s := Default()
	s.SetTerminal("Ghostty")
	if got := s.Terminals[runtime.GOOS]; got != "Ghostty" {
		t.Errorf("this platform = %q", got)
	}
	for platform, value := range s.Terminals {
		if platform != runtime.GOOS {
			t.Errorf("wrote %s = %q", platform, value)
		}
	}
}

func TestAnEmptyChoiceIsRemoved(t *testing.T) {
	s := Default()
	s.SetTerminal("Ghostty")
	s.SetTerminal("")
	if s.Terminal() != "" {
		t.Errorf("terminal = %q, want no choice", s.Terminal())
	}
	if s.Terminals != nil {
		t.Errorf("terminals = %#v, want nil", s.Terminals)
	}
}

func TestBadSettingsAreRefused(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"bad json", "{", "will not parse"},
		{"wrong version", `{"version":2}`, "version is 2"},
		{"bad platform", `{"version":1,"terminals":{"macos":"Ghostty"}}`, "not a platform"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(c.raw), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatal("bad settings were accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
		})
	}
}

func TestPathSitsNextToTheCommandList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OSHELLY_CONFIG", filepath.Join(dir, "oshelly.conf"))

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "settings.json"); path != want {
		t.Errorf("Path = %q, want %q", path, want)
	}
}
