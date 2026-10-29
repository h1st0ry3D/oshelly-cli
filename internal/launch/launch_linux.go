//go:build linux

package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// terminalCandidates is the fallback order on Linux, after the desktop's own
// default terminal and $TERMINAL have been tried.
//
// It is a list of names rather than a single guess because no standard place
// says which terminal is default. The first entries cover a Wayland desktop;
// the rest let a plain X session work.
var terminalCandidates = []string{
	"ghostty",
	"alacritty",
	"kitty",
	"foot",
	"wezterm",
	"konsole",
	"gnome-terminal",
	"xfce4-terminal",
	"xterm",
}

// Resolve reports which terminal would be used, without starting anything.
//
// It builds the same plan Start does, with an empty command, because the
// choice of terminal does not depend on the command. `doctor` uses this so a
// machine can be checked before the first click.
func Resolve(configured string) (Terminal, error) {
	plan, err := planFor("", configured, true, currentEnv())
	if err != nil {
		return Terminal{}, err
	}
	return Terminal{Path: plan.Program, How: plan.How}, nil
}

// Start opens the command in a terminal and returns immediately.
//
// The child is put in its own session so closing or killing the short-lived
// CLI cannot take the terminal with it, and its three standard streams are
// deliberately nil: they become /dev/null. A terminal or a launcher that
// inherits the panel's pipe would keep that pipe open for as long as the
// terminal lives, and the panel would wait for a command that had already
// been handed off.
func Start(command, configured string, keepOpen bool) (Terminal, error) {
	plan, err := planFor(command, configured, keepOpen, currentEnv())
	if err != nil {
		return Terminal{}, err
	}

	cmd := exec.Command(plan.Program, plan.Args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Terminal{}, fmt.Errorf("start %s: %w", plan.Program, err)
	}
	// Release rather than Wait: this process has no use for the exit status,
	// and waiting would make the launcher as long-lived as the terminal.
	_ = cmd.Process.Release()
	return Terminal{Path: plan.Program, How: plan.How}, nil
}

// planFor chooses a terminal and the arguments that run the command in it.
//
// The order is the environment override, then the saved choice, then the
// desktop's own launcher, then the standard default-terminal utility, then
// $TERMINAL, then the list. Each of those means "this is the terminal this
// machine uses" in a different way, and trying them in this order is how the
// answer stays right on Omarchy, on another Wayland desktop and on a bare X
// session.
func planFor(command, configured string, keepOpen bool, e env) (Plan, error) {
	script := wrapper(command, keepOpen)

	if override := strings.TrimSpace(e.getenv("OSHELLY_TERMINAL")); override != "" {
		prog, ok := e.look(override)
		if !ok {
			// Falling through would ignore the one setting that exists to
			// override this, and the user would see a terminal they did not
			// ask for with no clue why. A typo should be visible.
			return Plan{}, fmt.Errorf("OSHELLY_TERMINAL is set to %q, which is not an executable",
				override)
		}
		return Plan{Program: prog, Args: terminalArgs(filepath.Base(prog), script), How: "OSHELLY_TERMINAL"}, nil
	}

	if configured != "" {
		prog, ok := e.look(configured)
		if !ok {
			// A saved choice that no longer exists is a terminal the user
			// asked for, so it should be named rather than replaced by the
			// desktop default without a word.
			return Plan{}, fmt.Errorf("the saved terminal %q is not an executable", configured)
		}
		return Plan{Program: prog, Args: terminalArgs(filepath.Base(prog), script), How: "saved setting"}, nil
	}

	// Omarchy's launcher is the desktop's answer: it starts the terminal
	// through uwsm with the active terminal's working directory. It is a
	// script rather than a binary, and it is not in a normal PATH, so lookup
	// knows where it lives.
	if prog, ok := e.look("omarchy-launch-terminal"); ok {
		return Plan{Program: prog, Args: []string{"sh", "-c", script}, How: "omarchy-launch-terminal"}, nil
	}

	// xdg-terminal-exec is the cross-desktop standard for this. It reads the
	// user's default terminal from the desktop entry, so it is the right
	// answer on a machine that is not Omarchy.
	if prog, ok := e.look("xdg-terminal-exec"); ok {
		return Plan{Program: prog, Args: []string{"sh", "-c", script}, How: "xdg-terminal-exec"}, nil
	}

	if term := strings.TrimSpace(e.getenv("TERMINAL")); term != "" {
		if prog, ok := e.look(term); ok {
			return Plan{Program: prog, Args: terminalArgs(filepath.Base(prog), script), How: "$TERMINAL"}, nil
		}
		// A stale $TERMINAL is common in a session that outlived a package
		// change. Falling through to a terminal that is installed
		// is more useful than refusing to launch anything.
	}

	for _, name := range terminalCandidates {
		if prog, ok := e.look(name); ok {
			return Plan{Program: prog, Args: terminalArgs(name, script), How: name}, nil
		}
	}

	return Plan{}, ErrNoTerminal
}
