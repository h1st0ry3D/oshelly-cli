//go:build linux

package launch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testEnv is the selection order without a machine around it.
func testEnv(vars map[string]string, found map[string]string) env {
	return env{
		getenv: func(k string) string { return vars[k] },
		look: func(name string) (string, bool) {
			p, ok := found[name]
			return p, ok
		},
	}
}

func TestTheDesktopLauncherWinsOverTheFallbacks(t *testing.T) {
	e := testEnv(
		map[string]string{"TERMINAL": "ghostty"},
		map[string]string{
			"omarchy-launch-terminal": "/usr/share/omarchy/bin/omarchy-launch-terminal",
			"xdg-terminal-exec":       "/usr/bin/xdg-terminal-exec",
			"ghostty":                 "/usr/bin/ghostty",
		})

	plan, err := planFor("echo hi", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "omarchy-launch-terminal" {
		t.Errorf("chose %q, want the desktop launcher", plan.How)
	}
	if got := plan.Args; len(got) != 3 || got[0] != "sh" || got[1] != "-c" {
		t.Errorf("args = %#v, want sh -c <script>", got)
	}
	if !strings.Contains(plan.Args[2], "echo hi") {
		t.Errorf("script does not contain the command: %q", plan.Args[2])
	}
}

func TestXDGTerminalExecIsTheSecondChoice(t *testing.T) {
	e := testEnv(
		map[string]string{"TERMINAL": "ghostty"},
		map[string]string{
			"xdg-terminal-exec": "/usr/bin/xdg-terminal-exec",
			"ghostty":           "/usr/bin/ghostty",
		})

	plan, err := planFor("echo hi", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "xdg-terminal-exec" {
		t.Errorf("chose %q, want xdg-terminal-exec", plan.How)
	}
}

func TestTerminalIsUsedBeforeTheFallbackList(t *testing.T) {
	e := testEnv(
		map[string]string{"TERMINAL": "alacritty"},
		map[string]string{
			"alacritty": "/usr/bin/alacritty",
			"ghostty":   "/usr/bin/ghostty",
		})

	plan, err := planFor("echo hi", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "$TERMINAL" {
		t.Errorf("chose %q, want $TERMINAL", plan.How)
	}
	if got := plan.Args[0]; got != "-e" {
		t.Errorf("first arg = %q, want -e for alacritty", got)
	}
}

// A stale $TERMINAL is common in a session that outlived a package change.
// Falling through to one that is installed beats refusing to launch anything.
func TestAStaleTerminalFallsThrough(t *testing.T) {
	e := testEnv(
		map[string]string{"TERMINAL": "gone"},
		map[string]string{"foot": "/usr/bin/foot"})

	plan, err := planFor("echo hi", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "foot" {
		t.Errorf("chose %q, want foot", plan.How)
	}
}

func TestOSHELLYTerminalOverridesEverything(t *testing.T) {
	e := testEnv(
		map[string]string{"OSHELLY_TERMINAL": "/opt/my-term", "TERMINAL": "ghostty"},
		map[string]string{
			"/opt/my-term":            "/opt/my-term",
			"omarchy-launch-terminal": "/usr/share/omarchy/bin/omarchy-launch-terminal",
		})

	plan, err := planFor("echo hi", e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "OSHELLY_TERMINAL" {
		t.Errorf("chose %q, want the override", plan.How)
	}
}

// An override that cannot be found is an error rather than a silent fallback:
// the setting exists to choose the terminal, and ignoring a typo in it would
// be the most confusing possible outcome.
func TestAMissingOverrideIsAnError(t *testing.T) {
	e := testEnv(
		map[string]string{"OSHELLY_TERMINAL": "/opt/not-there"},
		map[string]string{"ghostty": "/usr/bin/ghostty"})

	_, err := planFor("echo hi", e)
	if err == nil {
		t.Fatal("a missing override was ignored")
	}
	if !strings.Contains(err.Error(), "/opt/not-there") {
		t.Errorf("error %q does not name the value that was not found", err)
	}
}

func TestNoTerminalIsReportedAsSuch(t *testing.T) {
	_, err := planFor("echo hi", testEnv(nil, nil))
	if !errors.Is(err, ErrNoTerminal) {
		t.Errorf("err = %v, want ErrNoTerminal", err)
	}
}

func TestEachTerminalGetsTheFlagItUses(t *testing.T) {
	script := "true\n"
	cases := []struct {
		base string
		want []string
	}{
		{"ghostty", []string{"-e", "sh", "-c", script}},
		{"alacritty", []string{"-e", "sh", "-c", script}},
		{"konsole", []string{"-e", "sh", "-c", script}},
		{"xterm", []string{"-e", "sh", "-c", script}},
		{"kitty", []string{"sh", "-c", script}},
		{"foot", []string{"sh", "-c", script}},
		{"xdg-terminal-exec", []string{"sh", "-c", script}},
		{"omarchy-launch-terminal", []string{"sh", "-c", script}},
		{"wezterm", []string{"start", "--", "sh", "-c", script}},
		{"gnome-terminal", []string{"--", "sh", "-c", script}},
		{"xfce4-terminal", []string{"-x", "sh", "-c", script}},
	}
	for _, c := range cases {
		t.Run(c.base, func(t *testing.T) {
			got := terminalArgs(c.base, script)
			if len(got) != len(c.want) {
				t.Fatalf("args = %#v, want %#v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("args = %#v, want %#v", got, c.want)
				}
			}
		})
	}
}

// The terminal has to stay open after the command, or a script that prints an
// error would close before the error could be read. The command itself has to
// stay first and unchanged, because it is the user's own line.
func TestTheWrapperKeepsTheTerminalOpen(t *testing.T) {
	script := holdScript("echo hi")
	if !strings.HasPrefix(script, "echo hi\n") {
		t.Errorf("the command is not the first line: %q", script)
	}
	for _, want := range []string{"oshelly_status=$?", "exec", "-i"} {
		if !strings.Contains(script, want) {
			t.Errorf("wrapper %q does not contain %q", script, want)
		}
	}
}

func TestLookupFindsAnExecutableOnPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oshelly-test-terminal")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, ok := lookup("oshelly-test-terminal")
	if !ok {
		t.Fatal("an executable on PATH was not found")
	}
	if got != path {
		t.Errorf("lookup = %q, want %q", got, path)
	}
}

func TestLookupRefusesANonExecutable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "oshelly-test-terminal"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if got, ok := lookup("oshelly-test-terminal"); ok {
		t.Errorf("lookup found %q, which is not executable", got)
	}
}
