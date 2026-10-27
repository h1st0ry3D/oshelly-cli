// Package config reads the list of shell commands the front ends offer.
//
// The file is deliberately smaller than a justfile. One command per line,
// written as `name: command`, and that is the whole format. There are no
// recipes, dependencies, aliases or variables, because the only thing this
// tool does is launch one command in a terminal when its name is picked.
//
// The file is line based rather than JSON for the same reason: it is meant to
// be typed, read and edited by hand. A line is a name, a colon and a command,
// and a line that starts with # is a comment.
package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Version is the shape of the file this package reads. It is written into the
// JSON listing rather than the config file: the config has no header and no
// metadata, so a version field in it would be ceremony. The listing needs one
// because a front end is allowed to be older than the CLI that answers it.
const Version = 1

// MaxNameLength caps a name. A name is a label on one row of a panel and one
// item of a menu, and a line of a thousand characters is a mistake rather than
// a label. It is refused rather than truncated, because truncating two long
// names could make them the same name.
const MaxNameLength = 80

// Entry is one name and the command it launches.
type Entry struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// Config is the file after it has been read.
type Config struct {
	// Path is where it was read from, or would be read from.
	Path string
	// Exists says whether the file is there. A missing file is not an error:
	// a machine that has never been configured has no commands, which is a
	// truthful answer and one the panel can draw.
	Exists  bool
	Entries []Entry
}

// Empty reports whether there is nothing to show.
func (c Config) Empty() bool { return len(c.Entries) == 0 }

// Find returns the command with this name.
//
// The comparison is exact after the same whitespace cleanup the parser does,
// so a name copied from the panel always matches the entry it came from.
func (c Config) Find(name string) (Entry, bool) {
	name = CleanName(name)
	for _, e := range c.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Names returns the configured names in file order, for a message that says
// what could have been meant.
func (c Config) Names() []string {
	out := make([]string, 0, len(c.Entries))
	for _, e := range c.Entries {
		out = append(out, e.Name)
	}
	return out
}

// Path is where the config lives on this machine.
//
// It is always ~/.config/oshelly/oshelly.conf unless XDG_CONFIG_HOME says
// otherwise, including on macOS. Using os.UserConfigDir would put it under
// ~/Library/Application Support on a Mac, and the point of a shared CLI is
// that the same file works on both machines.
//
// OSHELLY_CONFIG takes either the file, which must end in .conf, or the
// directory holding it. The extension is what tells the two readings apart
// when the path does not exist yet.
func Path() (string, error) {
	if v := strings.TrimSpace(os.Getenv("OSHELLY_CONFIG")); v != "" {
		v = expandHome(v)
		if strings.HasSuffix(strings.ToLower(v), ".conf") {
			return v, nil
		}
		return filepath.Join(v, "oshelly.conf"), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(expandHome(xdg), "oshelly", "oshelly.conf"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no home directory to find the config in: %w", err)
	}
	return filepath.Join(home, ".config", "oshelly", "oshelly.conf"), nil
}

// expandHome turns a leading ~ into the home directory.
//
// A shell expands that before the program sees it, but a variable set in a
// desktop session or a launch agent is passed literally. Accepting both is
// cheaper than a bug that only appears when the variable is set somewhere
// other than a terminal.
func expandHome(v string) string {
	if v != "~" && !strings.HasPrefix(v, "~/") {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return v
	}
	if v == "~" {
		return home
	}
	return filepath.Join(home, v[2:])
}

// Load reads the file. A file that is not there yields an empty config rather
// than an error, because "nothing configured yet" is a normal state and the
// panel should be able to say so.
//
// A file that is there but will not parse is an error. Carrying on with an
// empty list would look exactly like a file with no commands in it, and the
// line that is wrong is the only useful thing to say.
func Load(path string) (Config, error) {
	cfg := Config{Path: path, Entries: []Entry{}}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	cfg.Exists = true
	entries, err := Parse(raw)
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Entries = entries
	return cfg, nil
}

// Parse reads the file's contents.
//
// The first colon separates the name from the command, and everything after
// it is the command unchanged. That lets a command contain a colon, a # or
// anything else a shell understands. A name cannot contain a colon, which is
// the one restriction the syntax has.
func Parse(raw []byte) ([]Entry, error) {
	// A UTF-8 byte-order mark is invisible in an editor and would otherwise
	// become part of the first name, making that one command impossible to
	// launch by the name the panel displays.
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	entries := []Entry{}
	seen := make(map[string]int)

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	// A command can be long and the default 64 KiB line limit is not a rule
	// anyone wrote down. A megabyte is a generous ceiling that still refuses
	// to read a binary file as if it were a list.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		at := strings.IndexByte(text, ':')
		if at < 0 {
			return nil, fmt.Errorf("line %d: %q has no colon. Write it as `name: command`", line, text)
		}

		name := CleanName(text[:at])
		command := strings.TrimSpace(text[at+1:])
		if name == "" {
			return nil, fmt.Errorf("line %d: the name before the colon is empty", line)
		}
		if count := utf8.RuneCountInString(name); count > MaxNameLength {
			return nil, fmt.Errorf("line %d: the name is %d characters; keep it to %d or fewer",
				line, count, MaxNameLength)
		}
		if command == "" {
			return nil, fmt.Errorf("line %d: %q has no command after the colon", line, name)
		}

		if first, ok := seen[name]; ok {
			// Two entries with one name cannot be told apart by a click or by
			// `run`, so this is refused instead of picking one. The line
			// numbers are what makes the duplicate findable in the file.
			return nil, fmt.Errorf("line %d: %q is already the name on line %d, "+
				"and two commands with one name cannot be told apart", line, name, first)
		}
		seen[name] = line

		entries = append(entries, Entry{Name: name, Command: command})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// CleanName makes a name fit one line of one button.
//
// Runs of whitespace collapse and control characters go, because a name is a
// label read back off a hand-edited file. Everything else, including spaces,
// punctuation and non-ASCII letters, is kept: a command called `Restart audio`
// should read as those two words.
func CleanName(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
