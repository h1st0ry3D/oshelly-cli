package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseReadsOneCommandPerLine(t *testing.T) {
	raw := []byte("# a comment\n\nRestart audio: systemctl --user restart pipewire\nUpdate: omarchy update\n")
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Name: "Restart audio", Command: "systemctl --user restart pipewire"},
		{Name: "Update", Command: "omarchy update"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// The first colon is the separator. A command is shell text and a colon is
// ordinary in it, so splitting on the last one or on every one would break
// commands that were never ambiguous.
func TestOnlyTheFirstColonSeparatesNameFromCommand(t *testing.T) {
	got, err := Parse([]byte("Show path: printf '%s:%s\\n' \"$HOME\" /tmp\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if want := `printf '%s:%s\n' "$HOME" /tmp`; got[0].Command != want {
		t.Errorf("command = %q, want %q", got[0].Command, want)
	}
}

// A # inside a command belongs to the command. Only a line whose first
// non-space character is # is a comment, which is the same rule a shell uses.
func TestAHashInsideACommandIsNotAComment(t *testing.T) {
	got, err := Parse([]byte("Count: printf '###\\n'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Command != "printf '###\\n'" {
		t.Errorf("command = %q", got[0].Command)
	}
}

func TestAByteOrderMarkIsNotPartOfTheFirstName(t *testing.T) {
	got, err := Parse([]byte("\xEF\xBB\xBFRestart: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "Restart" {
		t.Errorf("name = %q, want %q", got[0].Name, "Restart")
	}
}

func TestNamesHaveTheirWhitespaceTidied(t *testing.T) {
	got, err := Parse([]byte("  Restart \t audio  : true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "Restart audio" {
		t.Errorf("name = %q, want %q", got[0].Name, "Restart audio")
	}
}

// Two commands with one name cannot be told apart by a click or by `run`, so
// the file is refused rather than one of them being chosen silently.
func TestDuplicateNamesAreRefusedWithBothLineNumbers(t *testing.T) {
	_, err := Parse([]byte("One: true\nTwo: false\nOne: true\n"))
	if err == nil {
		t.Fatal("a duplicate name parsed without an error")
	}
	for _, want := range []string{"line 3", "line 1", "One"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A name is a label on one row, not a paragraph. Refusing a very long one is
// clearer than truncating it into something the file does not say.
func TestAnOverlongNameIsRefused(t *testing.T) {
	long := strings.Repeat("a", MaxNameLength+1)
	_, err := Parse([]byte(long + ": true\n"))
	if err == nil {
		t.Fatal("an overlong name was accepted")
	}
	if !strings.Contains(err.Error(), "keep it to 80") {
		t.Errorf("error %q does not say the limit", err)
	}
}

func TestBadLinesSayWhichLineIsWrong(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"no colon", "Restart true\n", []string{"line 1", "no colon"}},
		{"empty name", ": true\n", []string{"line 1", "name before the colon is empty"}},
		{"empty command", "Restart:\n", []string{"line 1", "no command after the colon"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.raw))
			if err == nil {
				t.Fatal("parsed without an error")
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestFindUsesTheCleanedName(t *testing.T) {
	cfg := Config{Entries: []Entry{{Name: "Restart audio", Command: "true"}}}
	if _, ok := cfg.Find("  Restart   audio "); !ok {
		t.Fatal("a name typed with extra spaces did not find its entry")
	}
	if _, ok := cfg.Find("restart audio"); ok {
		t.Fatal("the lookup is case-insensitive, which it should not be")
	}
}

// A missing file is a normal state, not a failure: a machine that has never
// been configured has no commands and the panel should be able to say so.
func TestLoadOfAMissingFileIsEmptyAndNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oshelly.conf")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exists {
		t.Error("Exists is true for a file that is not there")
	}
	if !cfg.Empty() {
		t.Errorf("got %d entries, want none", len(cfg.Entries))
	}
	if cfg.Path != path {
		t.Errorf("Path = %q, want %q", cfg.Path, path)
	}
}

func TestLoadRefusesAFileItCannotParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oshelly.conf")
	if err := os.WriteFile(path, []byte("this line has no colon\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("a malformed file was accepted")
	}
}

func TestPathUsesTheHomeConfigDirectory(t *testing.T) {
	t.Setenv("OSHELLY_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/tester")

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/home/tester/.config/oshelly/oshelly.conf"; path != want {
		t.Errorf("Path = %q, want %q", path, want)
	}
}

func TestPathLetsXDGConfigHomeWin(t *testing.T) {
	t.Setenv("OSHELLY_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	t.Setenv("HOME", "/home/tester")

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/xdg/oshelly/oshelly.conf"; path != want {
		t.Errorf("Path = %q, want %q", path, want)
	}
}

// OSHELLY_CONFIG takes the file or the directory holding it. The .conf suffix
// is what tells the two apart before the path exists.
func TestPathAcceptsAFileOrADirectory(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	cases := []struct {
		value, want string
	}{
		{"/tmp/my.conf", "/tmp/my.conf"},
		{"/tmp/oshelly-config", "/tmp/oshelly-config/oshelly.conf"},
		{"/tmp/oshelly-config/", "/tmp/oshelly-config/oshelly.conf"},
		{"~/dropbox/oshelly.conf", "/home/tester/dropbox/oshelly.conf"},
		{"~/dropbox", "/home/tester/dropbox/oshelly.conf"},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			t.Setenv("OSHELLY_CONFIG", c.value)
			path, err := Path()
			if err != nil {
				t.Fatal(err)
			}
			if path != c.want {
				t.Errorf("Path = %q, want %q", path, c.want)
			}
		})
	}
}
