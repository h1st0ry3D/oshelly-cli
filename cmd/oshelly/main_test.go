package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// configFile writes a config and points the CLI at it.
func configFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oshelly.conf")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OSHELLY_CONFIG", path)
	t.Setenv("OSHELLY_TERMINAL", "/bin/true")
	t.Setenv("OSHELLY_EDITOR", "/bin/true")
	return path
}

func invoke(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestListJSONIsWhatTheFrontEndsRead(t *testing.T) {
	path := configFile(t, "Restart audio: systemctl --user restart pipewire\nUpdate: omarchy update\n")

	code, stdout, stderr := invoke("list", "--json")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}

	var got listing
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if got.Path != path {
		t.Errorf("path = %q, want %q", got.Path, path)
	}
	if !got.Exists {
		t.Error("exists is false for a file that is there")
	}
	if len(got.Commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(got.Commands))
	}
	if got.Commands[0].Name != "Restart audio" {
		t.Errorf("first command = %#v", got.Commands[0])
	}
	if got.Terminal != "OSHELLY_TERMINAL" {
		t.Errorf("terminal = %q, want the override", got.Terminal)
	}
}

// An empty list has to be [] rather than null: a QML repeater reading null
// draws nothing and then has no length, and the empty state would not appear.
func TestAnEmptyConfigStillPrintsAnEmptyArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oshelly.conf")
	t.Setenv("OSHELLY_CONFIG", path)
	t.Setenv("OSHELLY_TERMINAL", "/bin/true")

	code, stdout, stderr := invoke("list", "--json")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, `"commands": []`) {
		t.Errorf("missing an empty commands array:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"exists": false`) {
		t.Errorf("missing the missing-file marker:\n%s", stdout)
	}
}

func TestListForAPersonShowsNamesAndCommands(t *testing.T) {
	configFile(t, "Restart audio: systemctl --user restart pipewire\n")

	code, stdout, stderr := invoke("list")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"Restart audio", "systemctl --user restart pipewire", "config:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestListRefusesAConfigItCannotParse(t *testing.T) {
	configFile(t, "this has no colon\n")

	code, _, stderr := invoke("list")
	if code != codeUsage {
		t.Errorf("exit %d, want %d", code, codeUsage)
	}
	for _, want := range []string{"line 1", "no colon"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not contain %q: %q", want, stderr)
		}
	}
}

func TestRunLaunchesTheNamedCommand(t *testing.T) {
	configFile(t, "Restart audio: systemctl --user restart pipewire\n")

	code, stdout, stderr := invoke("run", "Restart audio")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Launched") {
		t.Errorf("stdout = %q, want a launch message", stdout)
	}
}

// The name is matched after the same whitespace cleanup the parser does, so a
// name copied out of the panel with a stray space still works.
func TestRunAcceptsTheCleanedName(t *testing.T) {
	configFile(t, "Restart audio: true\n")

	code, _, stderr := invoke("run", "  Restart   audio ")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestRunSaysWhatIsConfiguredWhenTheNameIsWrong(t *testing.T) {
	configFile(t, "Restart audio: true\nUpdate: true\n")

	code, _, stderr := invoke("run", "Restart")
	if code != codeUsage {
		t.Errorf("exit %d, want %d", code, codeUsage)
	}
	for _, want := range []string{"no command called", "Restart audio", "Update"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not contain %q: %q", want, stderr)
		}
	}
}

// A broken settings file must not hide the command list or the Terminal
// submenu. Choosing a terminal rewrites the file.
func TestListSurvivesBrokenSettings(t *testing.T) {
	path := configFile(t, "Restart: true\n")
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "settings.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := invoke("list", "--json")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, `"name": "Restart"`) {
		t.Errorf("the command list was hidden:\n%s", stdout)
	}
	if !strings.Contains(stderr, "settings not usable") {
		t.Errorf("stderr = %q, want the settings error", stderr)
	}
}

func TestTerminalChoiceIsStoredForThisPlatform(t *testing.T) {
	path := configFile(t, "Restart: true\n")

	code, stdout, stderr := invoke("terminal", "Ghostty")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "terminal = Ghostty") {
		t.Errorf("stdout = %q", stdout)
	}

	raw, err := os.ReadFile(filepath.Join(filepath.Dir(path), "settings.json"))
	if err != nil {
		t.Fatalf("the settings file was not written: %v", err)
	}
	if !strings.Contains(string(raw), `"`+runtime.GOOS+`": "Ghostty"`) {
		t.Errorf("settings do not record this platform:\n%s", raw)
	}

	code, stdout, stderr = invoke("terminal")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "terminal: Ghostty") {
		t.Errorf("stdout = %q", stdout)
	}

	code, stdout, stderr = invoke("list", "--json")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, `"terminalChoice": "Ghostty"`) {
		t.Errorf("list does not expose the choice:\n%s", stdout)
	}

	code, stdout, stderr = invoke("terminal", "default")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "system default") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestEditCreatesTheConfigAndOpensIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oshelly.conf")
	t.Setenv("OSHELLY_CONFIG", path)
	t.Setenv("OSHELLY_EDITOR", "/bin/true")

	code, stdout, stderr := invoke("edit")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Editing") {
		t.Errorf("stdout = %q, want an editing message", stdout)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config was not created: %v", err)
	}
	if !strings.Contains(string(raw), "One command per line") {
		t.Errorf("the new config does not explain its format:\n%s", raw)
	}
}

// Editing an existing file must not replace it. A config that will not parse
// is exactly the file someone needs to open.
func TestEditLeavesAnExistingConfigAlone(t *testing.T) {
	path := configFile(t, "this line is wrong\n")
	t.Setenv("OSHELLY_EDITOR", "/bin/true")

	code, _, stderr := invoke("edit")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "this line is wrong\n" {
		t.Errorf("the config was changed:\n%s", raw)
	}
}

func TestEditReportsAMissingEditorOverride(t *testing.T) {
	configFile(t, "Restart: true\n")
	t.Setenv("OSHELLY_EDITOR", "/opt/not-there")

	code, _, stderr := invoke("edit")
	if code != codeStartFailed {
		t.Errorf("exit %d, want %d", code, codeStartFailed)
	}
	if !strings.Contains(stderr, "OSHELLY_EDITOR") {
		t.Errorf("stderr = %q, want the setting named", stderr)
	}
}

func TestEditTakesNoArguments(t *testing.T) {
	configFile(t, "Restart: true\n")
	code, _, stderr := invoke("edit", "extra")
	if code != codeUsage {
		t.Errorf("exit %d, want %d", code, codeUsage)
	}
	if !strings.Contains(stderr, "edit takes no arguments") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestPathPrintsTheConfigLocation(t *testing.T) {
	path := configFile(t, "Restart: true\n")

	code, stdout, stderr := invoke("path")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != path {
		t.Errorf("path = %q, want %q", strings.TrimSpace(stdout), path)
	}
}

func TestDoctorReportsAReadyMachine(t *testing.T) {
	configFile(t, "Restart audio: true\n")

	code, stdout, stderr := invoke("doctor")
	if code != codeOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"commands: 1", "terminal:", "ready."} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestNoArgumentsPrintsUsage(t *testing.T) {
	code, _, stderr := invoke()
	if code != codeUsage {
		t.Errorf("exit %d, want %d", code, codeUsage)
	}
	if !strings.Contains(stderr, "oshelly - run a shell command") {
		t.Errorf("stderr is not usage: %q", stderr)
	}
}
