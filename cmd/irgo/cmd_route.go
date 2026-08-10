// Routing: one grammar, <noun> <verb> [target].
//
// The CLI used to mix shapes — `build ios` but `app install ios`, `doctor` but
// `tools remove`. Two grammars means two things to learn and no way to guess a
// command you have not seen. Every command is now a noun and a verb.
//
// The nouns are the things irgo acts on:
//
//	project   the repository you are in
//	app       what gets built, run, shipped and installed
//	tools     the toolchains on this machine
//	server    the development server
//
// There is deliberately no platform noun. ios, android and desktop are targets
// — `app build ios` — so a noun of the same name would mean two things.
package main

import (
	"fmt"
	"strings"
)

// route maps a noun and its verb to the code that runs it. Returns false when
// the pair is not a command, so the caller can report it against the noun's
// own verb list rather than a generic unknown-command message.
func route(noun, verb string, args []string) (error, bool) {
	// --help anywhere means help, and help must never do work. Without this,
	// `irgo app run android --help` fell through to the runner: it generated
	// assets, built the AAR, and launched the app on an emulator. Asking what
	// a command does should never be the thing that does it.
	if hasFlag(args, "--help", "-h") {
		printCommandHelp(noun, verb)
		return nil, true
	}

	switch noun {

	case "project":
		switch verb {
		case "new":
			// --check answers the question without writing anything, for a
			// repo that is meant to BE generated output.
			if hasFlag(args, "--check") {
				return runNewCheck(), true
			}
			if len(args) < 1 {
				return fmt.Errorf("usage: irgo project new <name>  (or \".\" for here)"), true
			}
			return newProject(args[0]), true
		case "clean":
			return runClean(hasFlag(args, "--all", "-a")), true
		case "upgrade":
			if hasFlag(args, "--check") {
				return runUpgradeCheck(), true
			}
			return runUpgrade(hasFlag(args, "--force"), hasFlag(args, "--diff")), true
		case "pin":
			return runPin(args), true
		case "ci":
			return runCI(hasFlag(args, "--force", "-f")), true
		case "assets":
			return ensureAssets(), true
		case "test":
			return runTest(), true
		case "config":
			return runConfig(args), true
		}

	case "app":
		target := appTarget(args)
		switch verb {
		case "build":
			if target == "all" && len(args) == 0 {
				return fmt.Errorf("usage: irgo app build <%s>", buildTargetList()), true
			}
			return runAppBuild(target, args), true
		case "run":
			if len(args) == 0 {
				return fmt.Errorf("usage: irgo app run <ios|android|desktop>"), true
			}
			return runAppRun(target, args), true
		case "package":
			if len(args) == 0 {
				return fmt.Errorf("usage: irgo app package <ios|android|macos|windows>"), true
			}
			return runAppPackage(args), true
		case "install":
			return runAppInstall(target), true
		case "remove":
			return runAppUninstall(target), true
		case "deploy":
			if target != "cloudflare" {
				return fmt.Errorf("usage: irgo app deploy cloudflare"), true
			}
			modulePath, err := getModulePath()
			if err != nil {
				return fmt.Errorf("could not determine module path: %w", err), true
			}
			return deployCloudflare(modulePath), true
		case "reviews":
			return reviewsCommand(args), true
		}

	case "tools":
		rest := args
		android := len(rest) > 0 && rest[0] == "android"
		if android {
			rest = rest[1:]
		}
		switch verb {
		case "install":
			if android {
				avd := defaultAVD
				for i := 0; i < len(rest)-1; i++ {
					if rest[i] == "--avd" {
						avd = rest[i+1]
					}
				}
				return installAndroidTools(hasFlag(rest, "--emulator", "-e"), avd), true
			}
			return installTools(), true
		case "remove":
			scope := ""
			if android {
				scope = "android"
			}
			return uninstallTools(scope, hasFlag(rest, "--all"),
				hasFlag(rest, "--yes", "-y"), hasFlag(rest, "--keep-jdk")), true
		case "doctor":
			switch {
			case android:
				return doctorAndroid(), true
			case hasFlag(rest, "--fix"):
				return runDoctorFix(), true
			default:
				return doctorHost(hasFlag(rest, "--strict")), true
			}
		}

	case "server":
		switch verb {
		case "dev":
			return runDev(), true
		case "serve", "start":
			return runServe(), true
		}

	}
	return nil, false
}

// appValueFlags are the flags that consume the argument after them, so a
// target search does not mistake a flag's value for the platform.
var appValueFlags = map[string]bool{"--team": true}

// appTarget picks the platform out of the arguments.
//
// It used to be args[0] and nothing else, which meant a flag written first —
// `app run --dev android`, the order most people reach for — left the target
// as "all" and failed with "unknown platform: all" while the platform was
// sitting right there in the arguments.
func appTarget(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if appValueFlags[a] {
				i++ // skip the value, it is not the target
			}
			continue
		}
		return a
	}
	return "all"
}

// renamed names where each removed command went. It does NOT dispatch: the old
// spellings are gone, and one grammar means one spelling for each thing. It
// exists so muscle memory and an old script get an answer instead of the whole
// usage screen and a guess.
var renamed = map[string]string{
	"new": "project new", "clean": "project clean", "upgrade": "project upgrade",
	"pin": "project pin", "ci": "project ci", "test": "project test",
	"assets": "project assets", "templ": "project assets",

	"build": "app build", "run": "app run",
	"package": "app package", "reviews": "app reviews",

	"dev": "server dev", "serve": "server serve",

	"install-tools": "tools install", "uninstall-tools": "tools remove",
	"doctor": "tools doctor",

	"ios": "app build ios, or project config ios.team",
}
