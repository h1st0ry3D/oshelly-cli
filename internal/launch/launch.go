// Package launch starts the external desktop programs the CLI hands work to:
// a terminal for a configured command, and an editor for the config file.
//
// The CLI does not run a command itself. It hands it to a terminal and
// returns, so the panel that asked is not blocked for as long as the command
// runs. That is also what makes an interactive command possible: a script that
// asks a question is an ordinary thing to launch from a bar.
//
// The same reasoning applies to editing: the CLI asks the desktop for its
// default editor and returns, so a terminal editor can open in a terminal
// while a GUI editor opens as a window, and this program does not have to know
// which one the user chose.
//
// These are desktop programs and need the session's environment to find their
// display, their Wayland socket and their D-Bus. This is the opposite of the
// odisplay CLI, which strips the environment because it runs hardware tools.
// The distinction is deliberate: nothing here decides what a program name
// resolves to inside the command, because the command is the user's own code.
package launch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoTerminal means no terminal program could be found. A terminal that was
// found and failed to start is a different error, because the fixes differ.
var ErrNoTerminal = errors.New("no terminal found")

// ErrNoEditor means no editor program could be found. A missing editor and a
// missing terminal have different fixes, so they are different errors.
var ErrNoEditor = errors.New("no editor found")

// ErrUnsupported is for a platform this has no way to open a terminal or an
// editor on.
var ErrUnsupported = errors.New("launching a terminal or editor is not supported on this platform")

// Program is an external program the CLI hands work to.
type Program struct {
	// Path is the absolute path that was found.
	Path string
	// How is a word for the mechanism, such as "xdg-terminal-exec" or "the
	// default .command handler". It is what `doctor` prints so a person can
	// see which candidate was chosen without reading the selection order.
	How string
}

// String is the form `doctor` prints.
func (p Program) String() string {
	if p.How == "" {
		return p.Path
	}
	return p.Path + " (" + p.How + ")"
}

// Terminal is the program that will open a configured command.
type Terminal = Program

// Editor is the program that will open the config file.
//
// How is a word for the mechanism, not the editor's own name: on Omarchy the
// program is omarchy-launch-config-editor, which chooses the user's editor and
// opens a terminal for a TUI one. Naming the launcher is more truthful than
// naming an editor this program never resolved.
type Editor = Program

// Plan is one external program invocation before it is started. It is used
// for both terminals and editors, which differ in the arguments they need and
// not in the shape of the call.
type Plan struct {
	Program string
	Args    []string
	How     string
}

// env is the two things selection needs from the outside world, passed in so
// the order can be tested without a terminal installed.
type env struct {
	getenv func(string) string
	look   func(string) (string, bool)
}

func currentEnv() env {
	return env{getenv: os.Getenv, look: lookup}
}

// holdScript wraps a command so the terminal stays open after it finishes.
//
// Without this a script that prints an error would close the terminal before
// the error could be read, which is the failure this wrapper prevents. The
// command is run first and unchanged; the status line and the interactive
// shell after it are what the person sees next.
//
// It runs under `sh` rather than the login shell because this is a wrapper,
// not the command: `sh` is on every machine and has the same syntax for the
// four lines below. The shell that opens afterwards is $SHELL, which is the
// user's own.
func holdScript(command string) string {
	return command + "\n" +
		"oshelly_status=$?\n" +
		"printf '\\n[oshelly] exited %s\\n' \"$oshelly_status\"\n" +
		"exec \"${SHELL:-/bin/sh}\" -i\n"
}

// lookup finds a program without trusting PATH alone.
//
// A menu bar app and a bar panel do not always start with the PATH a login
// shell has. PATH is searched first because it is what the session says, then
// the places a hand-installed or system terminal lives. The Omarchy
// directory is included because its launcher is the right terminal on that
// desktop even though it is not in a normal user's PATH.
func lookup(name string) (string, bool) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return executable(name)
	}

	dirs := filepath.SplitList(os.Getenv("PATH"))
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	dirs = append(dirs,
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/usr/share/omarchy/bin",
		"/opt/homebrew/bin",
	)

	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		if path, ok := executable(filepath.Join(dir, name)); ok {
			return path, true
		}
	}
	return "", false
}

// executable reports whether a path is a file this user can run.
func executable(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", false
	}
	return path, true
}

// openArgs is how `open` is told to run the script.
//
// Ghostty asks for confirmation every time a .command file is opened through
// its openFile handler, and there is no setting to turn that off. Passing the
// script through --initial-command keeps it out of that path: the argument is
// an option rather than a document, so AppKit never hands Ghostty a file to
// execute. The price is a new Ghostty instance, which quits when the window
// closes. The running instance is left alone.
func openArgs(app, scriptPath string) []string {
	if isGhostty(app) {
		appFlag := "-na"
		if strings.EqualFold(strings.TrimSpace(app), "com.mitchellh.ghostty") {
			appFlag = "-nb"
		}
		return []string{
			appFlag, app,
			"--args",
			"--window-save-state=never",
			"--quit-after-last-window-closed=true",
			"--initial-command=" + scriptPath,
		}
	}
	if app != "" {
		return []string{"-a", app, scriptPath}
	}
	return []string{scriptPath}
}

// isGhostty accepts an application name or a bundle path. The case varies
// between "Ghostty", "ghostty" and a path ending in "Ghostty.app".
func isGhostty(app string) bool {
	app = strings.TrimSpace(app)
	if strings.EqualFold(app, "com.mitchellh.ghostty") {
		return true
	}
	base := strings.TrimSuffix(app, ".app")
	return strings.EqualFold(filepath.Base(base), "Ghostty")
}

// terminalArgs is how a named terminal is told to run a command.
//
// Terminals disagree about this flag, and a wrong one is not a cosmetic
// problem: some of them interpret the next word as a title or a config file
// and exit, which would look like a launcher that does nothing. The names are
// the ones in terminalCandidates and the common $TERMINAL values.
func terminalArgs(base, script string) []string {
	switch base {
	case "wezterm":
		return []string{"start", "--", "sh", "-c", script}
	case "kitty", "foot", "xdg-terminal-exec", "omarchy-launch-terminal":
		// These take the command directly, with no -e. The last two are
		// launchers rather than terminals, but they are common enough as
		// OSHELLY_TERMINAL and $TERMINAL values to be worth naming.
		return []string{"sh", "-c", script}
	case "gnome-terminal":
		return []string{"--", "sh", "-c", script}
	case "xfce4-terminal":
		return []string{"-x", "sh", "-c", script}
	default:
		// ghostty, alacritty, konsole, xterm and most others use -e.
		return []string{"-e", "sh", "-c", script}
	}
}
