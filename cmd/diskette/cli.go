// The diskette binary doubles as its own client: `diskette view ...` run
// inside a terminal tab it spawned is a *second* invocation of this same
// executable, not a message sent to the running one directly — main()
// checks for a recognized subcommand before ever touching the TUI, and
// this file is everything that path needs. It talks back to the running
// diskette instance over the loopback IPC connection ipc.go's server
// side listens on, addressed by two environment variables set on this
// process only because it happens to be a child of that instance's own
// Terminal widget (see newTerminalContent):
//
//   - DISKETTE_IPC: the loopback address to dial (see startIPCServer).
//   - DISKETTE_TERMINAL_ID: which Terminal tab this shell is, so the
//     running instance knows which pane's sibling to act on.
//
// Neither is set for a `diskette` invoked anywhere else (a real shell,
// someone else's terminal emulator, ...); every command here treats
// that as an ordinary, expected error, not a crash.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/locales"
)

// runCLI runs a recognized subcommand and reports whether args[0] named
// one at all — main() falls through to starting the TUI when it didn't,
// so an ordinary `diskette` (or `diskette` given a path someone expects
// to open the app to, a possible future addition) is unaffected.
//
// Inside a diskette terminal tab specifically, that fallthrough is never
// right: a bare `diskette`, or any other args[0] this switch doesn't
// recognize, would otherwise start a second full-screen instance crammed
// into the small Terminal widget it's actually running inside of. Both
// cases print cliHelp's command list instead of falling through there.
func runCLI(args []string) (exitCode int, handled bool) {
	if len(args) == 0 {
		if insideDiskettePane() {
			return cliHelp(), true
		}
		return 0, false
	}
	switch args[0] {
	case "view":
		return cliView(args[1:]), true
	case "sync":
		return cliSync(args[1:]), true
	case "tag":
		return cliTagging(true, args[1:]), true
	case "untag":
		return cliTagging(false, args[1:]), true
	case "select":
		return cliSelect(args[1:]), true
	case "--version", "-v", "version":
		return cliVersion(), true
	default:
		if insideDiskettePane() {
			return cliHelp(), true
		}
		return 0, false
	}
}

// insideDiskettePane reports whether this process is running inside a
// shell diskette itself spawned in one of its own Terminal tabs.
func insideDiskettePane() bool {
	_, _, err := diskettePane()
	return err == nil
}

// cliVersion prints the build's version (see version.go) and exits,
// honoring the `--version`/`-v` convention from any shell — unlike
// cliHelp and the rest of this file, it's available whether or not this
// process happens to be running inside diskette's own Terminal tab.
func cliVersion() int {
	fmt.Println("diskette " + version)
	return 0
}

// cliHelp lists the commands available from inside a diskette terminal
// tab, printed in place of actually launching a nested diskette instance,
// in whichever language the running diskette instance was showing when
// it spawned this shell (see cliCatalog — this process is a separate
// invocation of the binary with no *Graphite.Application of its own to
// call T on, so DISKETTE_LOCALE is how that choice reaches it).
func cliHelp() int {
	cat := cliCatalog()
	t := func(key string) string {
		if v := cat[key]; v != "" {
			return v
		}
		return locales.English[key] // the same English-fallback rule Application.T itself follows
	}
	fmt.Println(t(locales.KeyCLIHelpIntro))
	fmt.Println()
	fmt.Println(t(locales.KeyCLIHelpCommands))
	fmt.Println()
	fmt.Println(t(locales.KeyCLIHelpView))
	fmt.Println(t(locales.KeyCLIHelpTag))
	fmt.Println(t(locales.KeyCLIHelpUntag))
	fmt.Println(t(locales.KeyCLIHelpSelect))
	fmt.Println(t(locales.KeyCLIHelpSync))
	return 0
}

// cliCatalog resolves $DISKETTE_LOCALE (set by newTerminalContent from
// the running instance's own Application.Locale) to one of diskette's
// four translated catalogs, falling back to English for an unset or
// unrecognized value — the same fallback locales.English itself already
// provides for any individual missing key.
func cliCatalog() Graphite.Catalog {
	switch Graphite.Locale(os.Getenv("DISKETTE_LOCALE")) {
	case Graphite.LocaleUkrainian:
		return locales.Ukrainian
	case Graphite.LocaleRussian:
		return locales.Russian
	case Graphite.LocaleDutch:
		return locales.Dutch
	default:
		return locales.English
	}
}

// diskettePane returns this process's IPC address and terminal ID, or an
// error naming whichever of the two environment variables is missing —
// the case where `diskette <cmd>` was run outside a diskette terminal
// tab at all.
func diskettePane() (addr, termID string, err error) {
	addr = os.Getenv("DISKETTE_IPC")
	termID = os.Getenv("DISKETTE_TERMINAL_ID")
	if addr == "" || termID == "" {
		return "", "", fmt.Errorf("not running inside a diskette terminal tab")
	}
	return addr, termID, nil
}

// cliRequest resolves the current pane, sends req.Cmd/req.Args, and
// prints a "diskette <cmd>: ..." error the same way every command here
// reports failure, so the shell's own $? and stderr both behave the way
// a well-behaved CLI tool's should.
func cliRequest(cmd string, args []string) int {
	addr, termID, err := diskettePane()
	if err != nil {
		fmt.Fprintf(os.Stderr, "diskette %s: %v\n", cmd, err)
		return 1
	}
	if err := sendIPC(addr, ipcRequest{TerminalID: termID, Cmd: cmd, Args: args}); err != nil {
		fmt.Fprintf(os.Stderr, "diskette %s: %v\n", cmd, err)
		return 1
	}
	return 0
}

// cliView implements `diskette view [path]`: navigate the other pane to
// path, or this shell's own current directory if none is given.
func cliView(args []string) int {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "diskette view:", err)
		return 1
	}
	return cliRequest("view", []string{abs})
}

// cliTagging implements `diskette tag`/`untag <name...>`: tag or untag
// entries in the other pane's current listing by name, so a shell
// one-liner's results (`ls *.log`, a find/grep pipeline, ...) become a
// real tagged selection ready for F5/F6 there.
func cliTagging(tag bool, names []string) int {
	if len(names) == 0 {
		verb := "tag"
		if !tag {
			verb = "untag"
		}
		fmt.Fprintf(os.Stderr, "usage: diskette %s <name...>\n", verb)
		return 1
	}
	cmd := "tag"
	if !tag {
		cmd = "untag"
	}
	return cliRequest(cmd, names)
}

// cliSelect implements `diskette select <name>`: move the cursor to one
// entry in the other pane's current listing without tagging it.
func cliSelect(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: diskette select <name>")
		return 1
	}
	return cliRequest("select", args)
}

// cliSync implements `diskette sync on|off`: prints a shell snippet to
// eval, following the same pattern tools like zoxide/direnv use for
// exactly this problem (a subprocess can't reach back into its parent
// shell's own state, so instead of trying to, it hands the shell a
// snippet to run in its own context). "on" installs a hook that calls
// `diskette view "$PWD"` after every prompt; "off" removes it. Not an
// IPC command at all — it never touches the running diskette instance
// directly, only the shell it's typed into — but still requires being
// run inside a diskette terminal tab, since the installed hook would be
// useless anywhere else.
func cliSync(args []string) int {
	if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
		fmt.Fprintln(os.Stderr, "usage: diskette sync on|off")
		return 1
	}
	if _, _, err := diskettePane(); err != nil {
		fmt.Fprintln(os.Stderr, "diskette sync:", err)
		return 1
	}

	kind := shellKind()
	if kind == "" {
		fmt.Fprintln(os.Stderr, "diskette sync: don't know how to hook this shell yet (only bash and zsh are supported)")
		return 1
	}
	fmt.Println(syncSnippet(kind, args[0] == "on"))
	return 0
}

// shellKind identifies which shell this process is running under, for
// syncSnippet's sake — set once at spawn time (see newTerminalContent),
// since the running diskette instance already knows exactly which shell
// it launched and shouldn't need to be guessed at again from $SHELL
// (which names the user's *default* shell, not necessarily this one).
func shellKind() string {
	return os.Getenv("DISKETTE_SHELL_KIND")
}

// syncSnippet returns the hook (on) or unhook (off) snippet for kind
// ("bash" or "zsh" — shellKind's only two possible non-empty values).
// _diskette_sync_hook's own errors are silenced (the terminal briefly
// showing "not running inside a diskette terminal tab" on every single
// prompt would be far more disruptive than a directory that just doesn't
// sync for a moment).
func syncSnippet(kind string, on bool) string {
	const hookBody = `_diskette_sync_hook() { diskette view "$PWD" >/dev/null 2>&1; }`

	if kind == "zsh" {
		if on {
			return hookBody + "\n" +
				"autoload -Uz add-zsh-hook\n" +
				"add-zsh-hook chpwd _diskette_sync_hook\n" +
				"_diskette_sync_hook"
		}
		return "add-zsh-hook -d chpwd _diskette_sync_hook 2>/dev/null\n" +
			"unset -f _diskette_sync_hook 2>/dev/null"
	}

	// bash: PROMPT_COMMAND has no built-in hook-list API the way zsh's
	// add-zsh-hook does, so this appends/removes the call by hand,
	// guarding against appending it twice if "sync on" is run again
	// while it's already installed.
	if on {
		return hookBody + "\n" +
			`case ";${PROMPT_COMMAND};" in ` +
			`*";_diskette_sync_hook;"*) ;; ` +
			`*) PROMPT_COMMAND="_diskette_sync_hook;${PROMPT_COMMAND}";; ` +
			"esac\n" +
			"_diskette_sync_hook"
	}
	return `PROMPT_COMMAND=$(printf '%s' "${PROMPT_COMMAND}" | sed 's/_diskette_sync_hook;//')` + "\n" +
		"unset -f _diskette_sync_hook 2>/dev/null"
}

// looksLikeShellPath reports whether path's base name matches name —
// used by newTerminalContent (main.go) to derive DISKETTE_SHELL_KIND
// from the shell path defaultShell() picked, tolerating a versioned name
// like "bash-5.2" or a full path like "/opt/homebrew/bin/zsh".
func looksLikeShellPath(path, name string) bool {
	return strings.Contains(strings.ToLower(filepath.Base(path)), name)
}
