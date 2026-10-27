//go:build linux

package launch

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ResolveEditor reports which editor would be used, without opening anything.
func ResolveEditor() (Editor, error) {
	plan, err := editorPlan("", currentEnv())
	if err != nil {
		return Editor{}, err
	}
	return Editor{Path: plan.Program, How: plan.How}, nil
}

// Edit opens the config file in the user's editor and returns immediately.
//
// Like Start, the child is detached and its streams are nil. An editor can
// stay open for an hour, and the panel that asked for it must not wait.
func Edit(path string) (Editor, error) {
	plan, err := editorPlan(path, currentEnv())
	if err != nil {
		return Editor{}, err
	}

	cmd := exec.Command(plan.Program, plan.Args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Editor{}, fmt.Errorf("start %s: %w", plan.Program, err)
	}
	_ = cmd.Process.Release()
	return Editor{Path: plan.Program, How: plan.How}, nil
}

// editorPlan chooses the editor launcher.
//
// Omarchy has a launcher for this: omarchy-launch-config-editor reads the
// default editor from Omarchy's defaults, sends a notification, and opens a
// terminal for a TUI editor or a window for a GUI one. This does not try to
// guess whether $EDITOR is nvim or VS Code.
//
// Outside Omarchy, xdg-open asks the desktop for the file's default
// application. That is not always a text editor, which is why the Omarchy
// launcher comes first there.
func editorPlan(path string, e env) (Plan, error) {
	if override := e.getenv("OSHELLY_EDITOR"); override != "" {
		prog, ok := e.look(override)
		if !ok {
			// A typo in the one setting that exists to choose the editor
			// should be visible, not silently replaced by xdg-open.
			return Plan{}, fmt.Errorf("OSHELLY_EDITOR is set to %q, which is not an executable", override)
		}
		return Plan{Program: prog, Args: []string{path}, How: "OSHELLY_EDITOR"}, nil
	}

	if prog, ok := e.look("omarchy-launch-config-editor"); ok {
		return Plan{Program: prog, Args: []string{path}, How: "omarchy-launch-config-editor"}, nil
	}
	if prog, ok := e.look("omarchy-launch-editor"); ok {
		return Plan{Program: prog, Args: []string{path}, How: "omarchy-launch-editor"}, nil
	}
	if prog, ok := e.look("xdg-open"); ok {
		return Plan{Program: prog, Args: []string{path}, How: "xdg-open"}, nil
	}

	return Plan{}, ErrNoEditor
}
