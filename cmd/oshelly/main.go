// Command oshelly launches one of the commands in ~/.config/oshelly/oshelly.conf
// in the user's default terminal.
//
// The file belongs to this program. The Omarchy bar panel and the macOS menu
// bar app read it through `list --json` and launch through `run`, so both front
// ends agree about what a name means and neither has to parse the file. That
// is the same division the odisplay projects use: one program owns the
// behaviour, and the front ends only draw a list and send a click.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/h1st0ry3D/oshelly-cli/internal/config"
	"github.com/h1st0ry3D/oshelly-cli/internal/launch"
	"github.com/h1st0ry3D/oshelly-cli/internal/settings"
)

// Exit codes. They are part of the contract with the panel and the menu bar
// app, so the two can tell a missing program apart from a bad name without
// reading the message. A terminal and an editor share a code because the front
// ends only need to show the message.
const (
	codeOK          = 0
	codeUsage       = 1
	codeMissing     = 2
	codeStartFailed = 3
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	if len(args) == 0 {
		usage(stderr)
		return codeUsage
	}

	switch args[0] {
	case "list":
		return cmdList(args[1:], out, stderr)
	case "run":
		return cmdRun(args[1:], out, stderr)
	case "edit":
		return cmdEdit(args[1:], out, stderr)
	case "terminal":
		return cmdTerminal(args[1:], out, stderr)
	case "doctor":
		return cmdDoctor(args[1:], out, stderr)
	case "path":
		return cmdPath(args[1:], out, stderr)
	case "-h", "--help", "help":
		usage(out)
		return codeOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return codeUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `oshelly - run a shell command from the bar

  oshelly list [--json]   show the configured commands
  oshelly run <name>      launch one in the default terminal
  oshelly edit            open the config in the default editor
  oshelly terminal [name] choose the terminal, or show the choice
  oshelly path            print the config file's location
  oshelly doctor          check the config, the terminal and the editor

The config is ~/.config/oshelly/oshelly.conf, one command per line:

  Restart audio: systemctl --user restart pipewire

A line that starts with # is a comment. The first colon separates the name
from the command, so a name cannot contain a colon. Everything after that
colon is the command, unchanged.

Exit codes:
  0  done
  1  usage or config problem
  2  no terminal or editor was found
  3  the terminal or editor was found but could not be started
`)
}

// loadSettings reads the terminal choices, or says why it could not.
func loadSettings(errOut io.Writer) (settings.Settings, bool) {
	path, err := settings.Path()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return settings.Default(), false
	}
	s, err := settings.Load(path)
	if err != nil {
		fmt.Fprintf(errOut, "settings not usable: %v\n", err)
		return settings.Default(), false
	}
	return s, true
}

// loadSettingsSoft is for `list`, which is how the front ends draw their menu.
// A broken settings file should not hide the command list or the Terminal
// submenu; choosing a terminal writes a fresh file. `run` and `doctor` still
// report the error rather than guessing.
func loadSettingsSoft(errOut io.Writer) settings.Settings {
	saved, ok := loadSettings(errOut)
	if !ok {
		return settings.Default()
	}
	return saved
}

// load reads the config, or says why it could not.
func load(errOut io.Writer) (config.Config, bool) {
	path, err := config.Path()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return config.Config{}, false
	}
	cfg, err := config.Load(path)
	if err != nil {
		// A file that will not parse is refused rather than treated as
		// empty: the difference between "no commands" and "a line that is
		// wrong" is the whole reason the parser has line numbers in its
		// errors.
		fmt.Fprintf(errOut, "config not usable: %v\n", err)
		return config.Config{}, false
	}
	return cfg, true
}

// listing is what `list --json` prints.
//
// It carries the path and whether the file exists as well as the commands, so
// a front end can say "edit this file" without knowing where the CLI looks.
// The command text is included because the config is the user's own file and
// a front end that wants to show what a row will run can do so without
// reading the file itself.
type listing struct {
	Version        int            `json:"version"`
	Path           string         `json:"path"`
	Exists         bool           `json:"exists"`
	Platform       string         `json:"platform"`
	Terminal       string         `json:"terminal,omitempty"`
	TerminalChoice string         `json:"terminalChoice,omitempty"`
	Commands       []config.Entry `json:"commands"`
}

func cmdList(args []string, out *bufio.Writer, errOut io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			fmt.Fprintf(errOut, "unknown option %q for list\n", a)
			return codeUsage
		}
	}

	cfg, ok := load(errOut)
	if !ok {
		return codeUsage
	}
	saved := loadSettingsSoft(errOut)

	// A missing terminal is reported but not fatal here. The list is still
	// worth reading, and `doctor` is where it becomes a failure.
	terminal, terminalErr := launch.Resolve(saved.Terminal())
	terminalName := ""
	if terminalErr == nil {
		terminalName = terminal.How
	}

	if asJSON {
		if cfg.Entries == nil {
			cfg.Entries = []config.Entry{}
		}
		raw, err := json.MarshalIndent(listing{
			Version:        config.Version,
			Path:           cfg.Path,
			Exists:         cfg.Exists,
			Platform:       runtime.GOOS,
			Terminal:       terminalName,
			TerminalChoice: saved.Terminal(),
			Commands:       cfg.Entries,
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(errOut, err)
			return codeUsage
		}
		fmt.Fprintln(out, string(raw))
		return codeOK
	}

	fmt.Fprintf(out, "config: %s\n", cfg.Path)
	if !cfg.Exists {
		fmt.Fprintln(out, "  (not created yet)")
	}
	fmt.Fprintf(out, "platform: %s\n", runtime.GOOS)
	printProgram(out, "terminal", terminal.String(), terminalErr)
	editor, editorErr := launch.ResolveEditor()
	printProgram(out, "editor", editor.String(), editorErr)

	if cfg.Empty() {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "No commands configured. Write one per line:")
		fmt.Fprintln(out, "    Restart audio: systemctl --user restart pipewire")
		return codeOK
	}

	fmt.Fprintln(out)
	for _, entry := range cfg.Entries {
		fmt.Fprintf(out, "  %s\n", entry.Name)
		fmt.Fprintf(out, "    %s\n", entry.Command)
	}
	return codeOK
}

// printProgram prints one of the external programs the CLI hands work to, or
// the reason there is not one. `list` and `doctor` say the same thing twice,
// so the wording lives here.
func printProgram(out *bufio.Writer, label, program string, err error) {
	if err == nil {
		fmt.Fprintf(out, "%s: %s\n", label, program)
		return
	}
	fmt.Fprintf(out, "%s: none (%v)\n", label, err)
}

func cmdRun(args []string, out *bufio.Writer, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "run takes one command name")
		return codeUsage
	}

	cfg, ok := load(errOut)
	if !ok {
		return codeUsage
	}

	saved, ok := loadSettings(errOut)
	if !ok {
		return codeUsage
	}

	entry, found := cfg.Find(args[0])
	if !found {
		if names := cfg.Names(); len(names) > 0 {
			fmt.Fprintf(errOut, "no command called %q; configured: %s\n",
				args[0], strings.Join(names, ", "))
		} else {
			fmt.Fprintf(errOut, "no command called %q, and nothing is configured in %s\n",
				args[0], cfg.Path)
		}
		return codeUsage
	}

	_, err := launch.Start(entry.Command, saved.Terminal())
	if errors.Is(err, launch.ErrNoTerminal) {
		fmt.Fprintln(errOut, "no terminal found: set OSHELLY_TERMINAL, "+
			"or install xdg-terminal-exec or a terminal this knows about.")
		return codeMissing
	}
	if err != nil {
		fmt.Fprintf(errOut, "could not launch %q: %v\n", entry.Name, err)
		return codeStartFailed
	}

	fmt.Fprintf(out, "Launched %q.\n", entry.Name)
	return codeOK
}

// cmdTerminal shows or records which terminal to use on this platform.
//
// The choice is per platform because "Ghostty" is an application name on macOS
// and a program name on Linux, and the two machines share one config folder.
func cmdTerminal(args []string, out *bufio.Writer, errOut io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(errOut, "terminal takes one name, or no argument to show the choice")
		return codeUsage
	}

	path, err := settings.Path()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return codeUsage
	}
	saved, loadErr := settings.Load(path)

	if len(args) == 0 {
		if loadErr != nil {
			fmt.Fprintf(errOut, "settings not usable: %v\n", loadErr)
			return codeUsage
		}
		choice := saved.Terminal()
		if choice == "" {
			choice = "system default"
		}
		fmt.Fprintf(out, "terminal: %s\n", choice)
		return codeOK
	}

	// Setting a value is also the escape hatch for a settings file that will
	// not parse: start from the defaults and write a valid file.
	if loadErr != nil {
		saved = settings.Default()
		saved.SetPath(path)
	}

	value := strings.TrimSpace(args[0])
	switch strings.ToLower(value) {
	case "default", "system":
		value = ""
	}
	saved.SetTerminal(value)
	if err := saved.Save(); err != nil {
		fmt.Fprintf(errOut, "could not save the terminal choice: %v\n", err)
		return codeUsage
	}

	if value == "" {
		fmt.Fprintf(out, "terminal = system default on %s\n", runtime.GOOS)
	} else {
		fmt.Fprintf(out, "terminal = %s on %s\n", value, runtime.GOOS)
	}
	return codeOK
}

// cmdEdit opens the config in the user's editor.
//
// It deliberately does not load the config first. A file that will not parse is
// exactly the file someone needs to edit, and refusing to open it because the
// parser rejected it would leave the line number in the error and no way to
// fix it from the panel.
func cmdEdit(args []string, out *bufio.Writer, errOut io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(errOut, "edit takes no arguments")
		return codeUsage
	}

	path, err := config.Path()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return codeUsage
	}
	if err := ensureConfigFile(path); err != nil {
		fmt.Fprintf(errOut, "could not open the config: %v\n", err)
		return codeUsage
	}

	if _, err := launch.Edit(path); err != nil {
		if errors.Is(err, launch.ErrNoEditor) {
			fmt.Fprintln(errOut, "no editor found: set OSHELLY_EDITOR, "+
				"or install an editor launcher this knows about.")
			return codeMissing
		}
		fmt.Fprintf(errOut, "could not open the config: %v\n", err)
		return codeStartFailed
	}

	fmt.Fprintf(out, "Editing %s.\n", path)
	return codeOK
}

// ensureConfigFile creates an empty, commented config the first time someone
// asks to edit it. Opening a nonexistent path in an editor is a worse first
// experience than opening a file that says what to put in it.
func ensureConfigFile(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s is a directory, not a file", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	starter := fmt.Sprintf(`# %s
#
# One command per line: name: command
# A line whose first non-space character is # is a comment.
#
# Restart audio: systemctl --user restart pipewire wireplumber
`, path)
	return os.WriteFile(path, []byte(starter), 0o644)
}

func cmdDoctor(args []string, out *bufio.Writer, errOut io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(errOut, "doctor takes no arguments")
		return codeUsage
	}

	path, err := config.Path()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return codeUsage
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(out, "config:   %s\n", path)
		fmt.Fprintf(out, "platform: %s\n", runtime.GOOS)
		fmt.Fprintf(out, "  NOT READY: %v\n", err)
		return codeUsage
	}
	saved, ok := loadSettings(errOut)
	if !ok {
		return codeUsage
	}

	fmt.Fprintf(out, "config:   %s\n", cfg.Path)
	if !cfg.Exists {
		fmt.Fprintln(out, "          (not created yet)")
	}
	fmt.Fprintf(out, "platform: %s\n", runtime.GOOS)
	fmt.Fprintf(out, "commands: %d\n", len(cfg.Entries))

	terminal, err := launch.Resolve(saved.Terminal())
	printProgram(out, "terminal", terminal.String(), err)
	if err != nil {
		fmt.Fprintln(out, "\nno terminal found. Set OSHELLY_TERMINAL, or install")
		fmt.Fprintln(out, "xdg-terminal-exec or a terminal this knows about.")
		return codeMissing
	}
	editor, editorErr := launch.ResolveEditor()
	printProgram(out, "editor", editor.String(), editorErr)
	if editorErr != nil {
		fmt.Fprintln(out, "\nno editor found. Set OSHELLY_EDITOR, or install")
		fmt.Fprintln(out, "omarchy-launch-config-editor, omarchy-launch-editor or xdg-open.")
		return codeMissing
	}

	if cfg.Empty() {
		fmt.Fprintln(out, "\nNothing is configured yet. Write one command per line:")
		fmt.Fprintln(out, "    Restart audio: systemctl --user restart pipewire")
		fmt.Fprintln(out, "\nready. Add a line and it will appear in the panel.")
		return codeOK
	}

	fmt.Fprintln(out, "\nready.")
	return codeOK
}

func cmdPath(args []string, out *bufio.Writer, errOut io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(errOut, "path takes no arguments")
		return codeUsage
	}
	path, err := config.Path()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return codeUsage
	}
	fmt.Fprintln(out, path)
	return codeOK
}
