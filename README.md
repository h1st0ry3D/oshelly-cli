# oshelly-cli

Run a shell command from the bar, in your terminal.

`oshelly` reads a small list of commands from
`~/.config/oshelly/oshelly.conf` and launches the one you name. It is the
shared program behind the [Omarchy bar panel][panel] and the
[macOS menu bar app][mac], so both front ends offer the same list and neither
one has to parse the file.

```
oshelly run "Restart audio"
```

## The config

One command per line, written the way a justfile recipe reads:

```sh
# ~/.config/oshelly/oshelly.conf

Restart audio: systemctl --user restart pipewire wireplumber
Update system: omarchy update
Tail the journal: journalctl -f
```

- The first `:` separates the name from the command. A command can contain
  colons; a name cannot.
- Everything after the colon is the command, unchanged.
- A line whose first non-space character is `#` is a comment. A `#` inside a
  command belongs to the command.
- Names may contain spaces. They are labels on a list, so `Restart audio` is a
  name and `restart-audio` is not required. A name is capped at 80 characters,
  because a label that long is a mistake rather than a label.
- Two entries cannot have the same name. They could not be told apart by a
  click or by `oshelly run`, so the file is refused with both line numbers.

There are no recipes, dependencies, aliases or variables. If a name needs a
dozen commands, put them in a script and make the line call the script. The
point of this file is to be small enough to read at a glance.

The file is read every time a command is launched. A change takes effect the
next time the panel or the menu is opened; nothing needs restarting.

## Commands

| Command | What it does |
| --- | --- |
| `oshelly list` | Show the configured commands, terminal and editor |
| `oshelly list --json` | The same, for a panel or a menu bar app to read |
| `oshelly run <name>` | Launch one command in the default terminal |
| `oshelly edit` | Open the config in the default editor |
| `oshelly terminal [name]` | Show or save the terminal used on this platform |
| `oshelly path` | Print the config file's location |
| `oshelly doctor` | Check the file parses and the terminal and editor can be found |

`oshelly run` returns as soon as the terminal has been started. It does not
wait for the command: a launcher that blocked for the length of an interactive
script would leave the bar waiting, and the terminal is the place to watch the
output anyway.

`oshelly edit` creates the config with a commented header if it does not exist
yet, and opens it without parsing it. A file that will not parse is exactly the
file someone needs to edit.

### Exit codes

These are the contract with the two front ends, so they can say something more
useful than "it failed":

| Code | Meaning |
| --- | --- |
| 0 | Done |
| 1 | Usage or config problem |
| 2 | No terminal or editor was found |
| 3 | A terminal or editor was found but could not be started |

## Which terminal

`oshelly` uses, in order:

1. `OSHELLY_TERMINAL`, if it is set. This is a session override, and a value
   that cannot be found is an error rather than a silent fallback.
2. The saved choice from `oshelly terminal`.
3. `omarchy-launch-terminal` on Omarchy. This starts the terminal through
   `uwsm`, in the active terminal's working directory.
4. `xdg-terminal-exec`, the cross-desktop default-terminal utility.
5. `$TERMINAL`, if it names something installed.
6. The first of `ghostty`, `alacritty`, `kitty`, `foot`, `wezterm`, `konsole`,
   `gnome-terminal`, `xfce4-terminal`, `xterm`.

### Saving a choice

```sh
oshelly terminal            # show the choice for this machine
oshelly terminal Ghostty    # macOS: an application name
oshelly terminal ghostty    # Linux: a program name
oshelly terminal default    # go back to choosing automatically
```

The choice is stored per platform in `~/.config/oshelly/settings.json`, next to
the command list. A Mac and a Linux machine can share the file because
"Ghostty" is an application name on one and a program name on the other.

On macOS, a saved application is passed to `open -a`, which hands the
`.command` file to that application. Ghostty runs it through its own file
handler, so no Ghostty-specific command line is needed. On Linux, the saved
name is looked up like any other terminal.

On Linux, `OSHELLY_TERMINAL` is an executable path or name. On macOS it is an
application name or bundle path, passed to `open -a`, and the default is
whatever macOS has registered for `.command` files, which is usually Terminal.

`oshelly doctor` prints which one was chosen:

```
config:   /home/you/.config/oshelly/oshelly.conf
platform: linux
commands: 3
terminal: /usr/bin/xdg-terminal-exec (xdg-terminal-exec)
editor:   /usr/share/omarchy/bin/omarchy-launch-config-editor (omarchy-launch-config-editor)

ready.
```

The terminal is left open when the command finishes, with a line saying its
exit status and then an interactive shell. A script that failed has to stay on
screen long enough to read the error, so the terminal does not close on its
own.

The command itself runs under `sh`, the one shell every machine has. That makes
a line behave the same on Linux and macOS and keeps the config from depending
on which login shell you use. The shell that opens afterwards is `$SHELL`, so a
command that needs a shell function or an alias should call that shell
explicitly:

```sh
Deploy: zsh -lc 'source ~/.zshrc && deploy'
```

## Which editor

`oshelly edit` uses, in order on Linux:

1. `OSHELLY_EDITOR`, if it is set. It is a program that takes the file path,
   and a value that cannot be found is an error rather than a silent fallback.
   Use a GUI editor here, or `omarchy-launch-editor` for a TUI editor, because
   a bare `nvim` has no terminal to draw in.
2. `omarchy-launch-config-editor` on Omarchy. It reads the default editor from
   Omarchy's defaults, sends a notification, and opens a terminal for a TUI
   editor or a window for a GUI one.
3. `omarchy-launch-editor`, for an Omarchy install that predates the config
   wrapper.
4. `xdg-open`, the desktop's own default application for the file.

On macOS the default is `open -t`, which asks Launch Services for the default
text editor. `OSHELLY_EDITOR` names an application there, the same way
`OSHELLY_TERMINAL` does, and is passed to `open -a`.

`oshelly edit` is what the Omarchy panel's header **Edit** button and the macOS
menu's **Edit config** item call, so both open the same file the same way.

## Config location

`$XDG_CONFIG_HOME/oshelly/oshelly.conf`, which is
`~/.config/oshelly/oshelly.conf` unless you set it. On macOS it is the same
`~/.config/...` path, not `~/Library/Application Support`, so one copy of the
file works on both machines.

`OSHELLY_CONFIG` overrides it and takes either the file, which must end in
`.conf`, or the directory holding it:

```sh
OSHELLY_CONFIG=~/Dropbox/oshelly.conf oshelly list
OSHELLY_CONFIG=~/Dropbox/oshelly oshelly list
```

The terminal choice lives in `settings.json` in the same directory, so moving
`OSHELLY_CONFIG` moves both files.

## Install

Needs Go 1.22 or newer. There is nothing to download alongside it.

```sh
git clone https://github.com/h1st0ry3D/oshelly-cli
cd oshelly-cli

go build -trimpath -o ~/.local/bin/oshelly ./cmd/oshelly
```

`/usr/local/bin` works too. The Omarchy panel looks in `/usr/local/bin` first
and `~/.local/bin` second, so either is found with no configuration. The macOS
app looks in those two and in `/opt/homebrew/bin`.

Then write the file. There is a commented starting point in
[`examples/oshelly.conf`](examples/oshelly.conf):

```sh
mkdir -p ~/.config/oshelly
cp examples/oshelly.conf ~/.config/oshelly/oshelly.conf
$EDITOR ~/.config/oshelly/oshelly.conf
```

and check it:

```sh
oshelly doctor
oshelly list
```

### macOS

The same binary builds and runs:

```sh
go build -trimpath -o ~/.local/bin/oshelly ./cmd/oshelly
```

`oshelly run` asks `open` for the default `.command` handler. To force a
terminal, for example iTerm:

```sh
OSHELLY_TERMINAL=iTerm oshelly run "Restart audio"
```

### Updating

```sh
cd oshelly-cli
git pull
go build -trimpath -o ~/.local/bin/oshelly ./cmd/oshelly
```

A panel or a menu bar app in front of it picks up the new binary the next time
it runs it. Only the front end itself needs restarting when its own files
change.

## Safety

This file is executable code, by design. A line can run anything you can run
in a terminal, with your environment, your files and your permissions. Keep the
file writable only by you, and treat a change to it the way you would treat a
change to `.zshrc`.

The two front ends run no shell of their own. They pass a name to `oshelly`,
which passes an argv array to the terminal; the command text is never assembled
by string concatenation at the caller. A name containing a quote or a space
cannot become a second command on the way.

## Development

```sh
go test ./...
GOOS=darwin go vet ./...
go test -race ./...
```

The parser, the terminal-selection order and the editor-selection order are
tested without a desktop. The `list --json` shape is tested in `cmd/oshelly`,
because it is the contract the QML panel and the menu bar app read.

## License

MIT

[panel]: https://github.com/h1st0ry3D/oshelly
[mac]: https://github.com/h1st0ry3D/oshelly-mac
