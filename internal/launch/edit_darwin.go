//go:build darwin

package launch

import (
	"os"
	"os/exec"
	"strings"
)

// ResolveEditor reports how the config would be opened, without opening it.
func ResolveEditor() (Editor, error) {
	if _, err := os.Stat(open); err != nil {
		return Editor{}, err
	}
	if app := strings.TrimSpace(os.Getenv("OSHELLY_EDITOR")); app != "" {
		return Editor{Path: open, How: "open -a " + app}, nil
	}
	return Editor{Path: open, How: "the default text editor"}, nil
}

// Edit opens the config file in the default text editor.
//
// macOS has a direct spelling for this: `open -t` asks Launch Services for the
// default text editor. Opening the file without -t uses the handler for the
// .conf extension, which may not exist.
//
// OSHELLY_EDITOR names an application, the same way OSHELLY_TERMINAL does:
// `OSHELLY_EDITOR=BBEdit` becomes `open -a BBEdit <path>`.
func Edit(path string) (Editor, error) {
	editor, err := ResolveEditor()
	if err != nil {
		return Editor{}, err
	}

	args := []string{}
	if app := strings.TrimSpace(os.Getenv("OSHELLY_EDITOR")); app != "" {
		args = append(args, "-a", app)
	} else {
		args = append(args, "-t")
	}
	args = append(args, path)

	cmd := exec.Command(open, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Editor{}, &OpenError{Args: args, Output: strings.TrimSpace(string(out)), Err: err}
	}
	return editor, nil
}
