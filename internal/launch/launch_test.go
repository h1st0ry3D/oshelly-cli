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

	plan, err := planFor("echo hi", "", true, e)
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

	plan, err := planFor("echo hi", "", true, e)
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

	plan, err := planFor("echo hi", "", true, e)
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

	plan, err := planFor("echo hi", "", true, e)
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

	plan, err := planFor("echo hi", "", true, e)
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

	_, err := planFor("echo hi", "", true, e)
	if err == nil {
		t.Fatal("a missing override was ignored")
	}
	if !strings.Contains(err.Error(), "/opt/not-there") {
		t.Errorf("error %q does not name the value that was not found", err)
	}
}

// A saved choice is used before the desktop default, because it is the answer
// the user asked for.
func TestASavedTerminalWinsOverTheDesktopDefault(t *testing.T) {
	e := testEnv(nil, map[string]string{
		"ghostty":                 "/usr/bin/ghostty",
		"omarchy-launch-terminal": "/usr/share/omarchy/bin/omarchy-launch-terminal",
	})

	plan, err := planFor("echo hi", "ghostty", true, e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "saved setting" {
		t.Errorf("chose %q, want the saved setting", plan.How)
	}
	if plan.Program != "/usr/bin/ghostty" {
		t.Errorf("program = %q", plan.Program)
	}
}

// The environment override is for a single session and wins over the saved
// choice, so a test or a temporary run can change one invocation.
func TestTheEnvironmentOverrideWinsOverTheSavedChoice(t *testing.T) {
	e := testEnv(
		map[string]string{"OSHELLY_TERMINAL": "foot"},
		map[string]string{
			"foot":    "/usr/bin/foot",
			"ghostty": "/usr/bin/ghostty",
		})

	plan, err := planFor("echo hi", "ghostty", true, e)
	if err != nil {
		t.Fatal(err)
	}
	if plan.How != "OSHELLY_TERMINAL" {
		t.Errorf("chose %q, want the environment override", plan.How)
	}
}

func TestAMissingSavedTerminalNamesTheChoice(t *testing.T) {
	_, err := planFor("echo hi", "ghostty", true, testEnv(nil, nil))
	if err == nil {
		t.Fatal("a missing saved terminal was ignored")
	}
	if !strings.Contains(err.Error(), "ghostty") {
		t.Errorf("error %q does not name the choice", err)
	}
}

func TestNoTerminalIsReportedAsSuch(t *testing.T) {
	_, err := planFor("echo hi", "", true, testEnv(nil, nil))
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
	script := wrapper("echo hi", true)
	if !strings.HasPrefix(script, "echo hi\n") {
		t.Errorf("the command is not the first line: %q", script)
	}
	for _, want := range []string{"oshelly_status=$?", "exec", "-i"} {
		if !strings.Contains(script, want) {
			t.Errorf("wrapper %q does not contain %q", script, want)
		}
	}
}

// With the close toggle on, the wrapper ends the shell instead of opening a
// new one. The clean exit is what lets a terminal profile that closes on a
// clean exit close the window.
func TestTheWrapperCanCloseTheTerminal(t *testing.T) {
	script := wrapper("echo hi", false)
	if !strings.HasPrefix(script, "echo hi\n") {
		t.Errorf("the command is not the first line: %q", script)
	}
	if strings.Contains(script, "exec") {
		t.Errorf("the close wrapper still opens a shell: %q", script)
	}
	if !strings.Contains(script, "exit 0") {
		t.Errorf("the close wrapper does not exit: %q", script)
	}
}

func TestTheCloseSettingReachesTheTerminalScript(t *testing.T) {
	e := testEnv(nil, map[string]string{"xdg-terminal-exec": "/usr/bin/xdg-terminal-exec"})
	plan, err := planFor("echo hi", "", false, e)
	if err != nil {
		t.Fatal(err)
	}
	script := plan.Args[2]
	if strings.Contains(script, "exec") {
		t.Errorf("script still opens a shell: %q", script)
	}
	if !strings.Contains(script, "exit 0") {
		t.Errorf("script does not close: %q", script)
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

// Ghostty must not be handed a bare .command path, because its openFile handler
// asks for confirmation every time. The path travels inside --initial-command.
func TestGhosttyUsesInitialCommandInsteadOfOpeningTheFile(t *testing.T) {
	script := "/private/var/folders/x/T/oshelly-1/run.command"
	got := openArgs("Ghostty", script)
	want := []string{
		"-na", "Ghostty",
		"--args",
		"--window-save-state=never",
		"--quit-after-last-window-closed=true",
		"--initial-command=" + script,
	}
	if len(got) != len(want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", got, want)
		}
	}
	for _, arg := range got {
		if arg == script {
			t.Error("the script is a bare argument, which opens the Ghostty confirmation dialog")
		}
	}
}

func TestGhosttyBundleIDUsesTheBundleFlag(t *testing.T) {
	got := openArgs("com.mitchellh.ghostty", "/tmp/run.command")
	if len(got) < 2 || got[0] != "-nb" || got[1] != "com.mitchellh.ghostty" {
		t.Errorf("args = %#v, want -nb with the bundle id", got)
	}
}

func TestOtherApplicationsGetTheScriptPath(t *testing.T) {
	script := "/private/var/folders/x/T/oshelly-1/run.command"
	got := openArgs("iTerm", script)
	if len(got) != 3 || got[0] != "-a" || got[1] != "iTerm" || got[2] != script {
		t.Errorf("args = %#v", got)
	}
}

func TestTheDefaultHandlerGetsJustTheScriptPath(t *testing.T) {
	got := openArgs("", "/tmp/run.command")
	if len(got) != 1 || got[0] != "/tmp/run.command" {
		t.Errorf("args = %#v", got)
	}
}

func TestGhosttyNamesAreRecognised(t *testing.T) {
	cases := map[string]bool{
		"Ghostty":                   true,
		"ghostty":                   true,
		"Ghostty.app":               true,
		"/Applications/Ghostty.app": true,
		"/Applications/Ghostty":     true,
		"com.mitchellh.ghostty":     true,
		"iTerm":                     false,
		"":                          false,
	}
	for name, want := range cases {
		if got := isGhostty(name); got != want {
			t.Errorf("isGhostty(%q) = %v, want %v", name, got, want)
		}
	}
}
