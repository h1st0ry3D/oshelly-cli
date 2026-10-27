//go:build darwin

package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// open is part of macOS and is the only way to start the user's default
// terminal without knowing which terminal that is.
const open = "/usr/bin/open"

// Resolve reports how a command would be opened, without writing or starting
// anything. `doctor` uses this.
func Resolve() (Terminal, error) {
	if _, err := os.Stat(open); err != nil {
		return Terminal{}, err
	}
	if app := strings.TrimSpace(os.Getenv("OSHELLY_TERMINAL")); app != "" {
		return Terminal{Path: open, How: "open -a " + app}, nil
	}
	return Terminal{Path: open, How: "the default .command handler"}, nil
}

// Start writes the command into a .command file and lets the default handler
// for that file open it.
//
// macOS has no system-wide "default terminal" setting to read, but it does
// have a default application for .command files, which is exactly the same
// question: what should run a shell script? `open` asks Launch Services, so
// Terminal, iTerm or anything else registered for the type is what the user
// gets. OSHELLY_TERMINAL overrides it with an application name or a bundle
// path when that default is not the wanted one.
//
// The file is left in a temporary directory. Deleting it immediately would be
// a race with the terminal that has not read it yet, and the OS cleans up its
// own temporary directory.
func Start(command string) (Terminal, error) {
	terminal, err := Resolve()
	if err != nil {
		return Terminal{}, err
	}

	dir, err := os.MkdirTemp("", "oshelly-*")
	if err != nil {
		return Terminal{}, err
	}
	path := filepath.Join(dir, "run.command")
	script := "#!/bin/sh\n" + holdScript(command)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return Terminal{}, err
	}

	args := []string{}
	if app := strings.TrimSpace(os.Getenv("OSHELLY_TERMINAL")); app != "" {
		args = append(args, "-a", app)
	}
	args = append(args, path)

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
