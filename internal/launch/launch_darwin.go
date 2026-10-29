//go:build darwin

package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// open is part of macOS and is the only supported way to start a terminal
// from a process like this one, with or without naming the application.
const open = "/usr/bin/open"

// Resolve reports how a command would be opened, without writing or starting
// anything. `doctor` uses this.
func Resolve(configured string) (Terminal, error) {
	if _, err := os.Stat(open); err != nil {
		return Terminal{}, err
	}
	app := strings.TrimSpace(os.Getenv("OSHELLY_TERMINAL"))
	if app == "" {
		app = strings.TrimSpace(configured)
	}
	if app != "" {
		if isGhostty(app) {
			return Terminal{Path: open, How: "Ghostty, new instance"}, nil
		}
		return Terminal{Path: open, How: "open -a " + app}, nil
	}
	return Terminal{Path: open, How: "the default .command handler"}, nil
}

// Start writes the command into a .command file and asks `open` to run it.
//
// macOS has no system-wide "default terminal" setting to read, but it does
// have a default application for .command files. `open` asks Launch Services,
// so Terminal, iTerm or anything else registered for the type is what the user
// gets. A saved choice or OSHELLY_TERMINAL names an application instead.
//
// Naming an application gets `open -a <app> <script>`, which hands the file to
// that application. Ghostty is the exception: see openArgs, because its file
// handler asks for confirmation every time. Ghostty's own CLI cannot start a
// terminal on macOS, so `open` is the supported path either way.
//
// The file is left in a temporary directory. Deleting it immediately would be
// a race with the terminal that has not read it yet, and the OS cleans up its
// own temporary directory.
func Start(command, configured string, keepOpen bool) (Terminal, error) {
	terminal, err := Resolve(configured)
	if err != nil {
		return Terminal{}, err
	}

	dir, err := os.MkdirTemp("", "oshelly-*")
	if err != nil {
		return Terminal{}, err
	}
	path := filepath.Join(dir, "run.command")
	script := "#!/bin/sh\n" + wrapper(command, keepOpen)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return Terminal{}, err
	}

	app := strings.TrimSpace(os.Getenv("OSHELLY_TERMINAL"))
	if app == "" {
		app = strings.TrimSpace(configured)
	}
	args := openArgs(app, path)

	cmd := exec.Command(open, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// `open` is the part that knows whether the handler exists, so this
		// is where a missing terminal becomes a message rather than a menu
		// item that appears to have worked.
		return Terminal{}, &OpenError{Args: args, Output: strings.TrimSpace(string(out)), Err: err}
	}
	return terminal, nil
}

// OpenError carries what `open` said, because its own error is always the
// same unhelpful "exit status 1".
type OpenError struct {
	Args   []string
	Output string
	Err    error
}

func (e *OpenError) Error() string {
	what := strings.Join(e.Args, " ")
	if e.Output == "" {
		return open + " " + what + ": " + e.Err.Error()
	}
	return open + " " + what + ": " + e.Output
}

func (e *OpenError) Unwrap() error { return e.Err }
