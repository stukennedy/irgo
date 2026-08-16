// irgo — one Go codebase for web, desktop, iOS and Android.
//
// The CLI has one grammar: <noun> <verb> [target]. The nouns are the things
// irgo acts on — project, app, tools, server, ios — and filenames follow the
// same nouns, so a command and the code behind it are findable from each
// other:
//
//	main.go                 argument handling only
//	cmd_route.go            the noun/verb table
//	help.go                 the help text
//	cmd_<noun>.go           a noun and its verbs
//	cmd_<noun>_<verb>.go    one verb, when it is large enough to stand alone
//	<noun>_<detail>.go      how that noun works: app_ios_build.go,
//	                        tools_android_sdk.go, app_macos_package.go
//	util.go                 shared helpers
//
// A filename must never end in a GOOS name. build_ios.go, cmd_ios.go and
// tools_android.go each compiled only for that GOOS and vanished from the
// package with no error — three times, which is why filenames_test.go now
// fails on it rather than a comment asking nicely.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
)

// version is what the CLI reports and what artifact stamps are compared
// against. It is resolved from the build rather than hardcoded: `go tool irgo`
// builds whatever go.mod pins, so a constant here would describe a release
// nobody is necessarily running — as it did, claiming 0.4.0 for every fork
// build. See cliVersion.
var version = cliVersion()

// fallbackVersion is used only when a binary carries no build information,
// which happens if it was assembled outside the module system.
const fallbackVersion = "0.4.0"

// cliVersion reports the module version this binary was built from, falling
// back to the VCS revision for a local checkout — which is exactly the case
// `irgo project pin --local` creates, and worth naming so a surprising result is
// traceable to a working tree rather than a release.
func cliVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return fallbackVersion
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	dirty := ""
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-modified"
			}
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		return "devel-" + rev + dirty
	}
	return fallbackVersion + "-devel"
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error

	// -C <dir> runs the command somewhere else, like make.
	//
	// Every irgo command acts on the project in the working directory, which
	// means deploying a second project in the same repository — the
	// documentation site is one — meant cd'ing first. A directory is not a new
	// command, so it is a flag rather than a noun: `irgo -C docs app deploy
	// cloudflare` is the same code path as running it from inside docs.
	args := os.Args[1:]
	if len(args) >= 2 && (args[0] == "-C" || args[0] == "--chdir") {
		if err := os.Chdir(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: -C %s: %v\n", args[1], err)
			os.Exit(1)
		}
		args = args[2:]
	}
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// One grammar, no exceptions: <noun> <verb> [target].
	noun, verb, rest := args[0], "", args[1:]
	if len(args) > 1 {
		verb, rest = args[1], args[2:]
	}

	switch noun {
	case "version", "-v", "--version":
		fmt.Printf("irgo %s\n", version)
		if r := projectReplacement(); r != "" {
			fmt.Printf("  running: %s\n", r)
		}
	case "help", "-h", "--help":
		// `irgo help`, `irgo help app`, `irgo help app run` — the same grammar
		// as the commands themselves.
		if hasFlag(args[1:], "--json") {
			printCommandsJSON()
			break
		}
		switch len(args) {
		case 1:
			printUsage()
		case 2:
			printCommandHelp(args[1], "")
		default:
			printCommandHelp(args[1], args[2])
		}
	default:
		var handled bool
		err, handled = route(noun, verb, rest)
		if !handled {
			if verbs := nounVerbs(noun); len(verbs) > 0 {
				if verb == "" {
					err = fmt.Errorf("usage: irgo %s <%s>", noun, strings.Join(verbs, "|"))
				} else {
					err = fmt.Errorf("unknown command: irgo %s %s\n  %s accepts: %s",
						noun, verb, noun, strings.Join(verbs, ", "))
				}
			} else if now, moved := renamed[noun]; moved {
				err = fmt.Errorf("`irgo %s` was removed — it is now `irgo %s`", noun, now)
			} else {
				fmt.Printf("Unknown command: %s\n", noun)
				printUsage()
				os.Exit(1)
			}
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
