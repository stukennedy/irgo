package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// captureStdout runs f and returns what it printed.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	f()
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestEveryCommandHasHelp fails when a command exists with nothing to say about
// it. Help drifted from the CLI before precisely because nothing connected the
// two: commands were renamed and the help kept describing the old ones, down to
// files that no longer shipped.
func TestEveryCommandHasHelp(t *testing.T) {
	for _, noun := range nouns {
		verbs := nounVerbs(noun)
		for _, verb := range verbs {
			out := captureStdout(t, func() { printCommandHelp(noun, verb) })
			if strings.Contains(out, "No help for") {
				t.Errorf("irgo %s %s has no help", noun, verb)
				continue
			}
			// The first line names the command, so a topic wired to the wrong
			// key is caught too rather than silently answering for its
			// neighbour.
			want := "irgo " + noun + " " + verb
			if first, _, _ := strings.Cut(out, "\n"); !strings.HasPrefix(first, want) {
				t.Errorf("irgo %s %s: help starts with %q, want it to start with %q",
					noun, verb, first, want)
			}
		}
	}
}

// TestHelpForANounListsItsVerbs checks the middle level: `irgo help app` should
// answer with app's verbs rather than falling through to the whole CLI.
func TestHelpForANounListsItsVerbs(t *testing.T) {
	for _, noun := range nouns {
		verbs := nounVerbs(noun)
		out := captureStdout(t, func() { printCommandHelp(noun, "") })
		for _, verb := range verbs {
			if !strings.Contains(out, "irgo "+noun+" "+verb) {
				t.Errorf("irgo help %s does not mention %s", noun, verb)
			}
		}
	}
}

// TestUsageIndexListsEveryCommand keeps the front page honest.
//
// The index is prose while the commands come from a table, so it drifts: `app
// deploy cloudflare` shipped working and undocumented, and `app build` went on
// listing targets that no longer matched what it accepted. The detail pages
// were right both times — it was the first screen, which is the only one most
// people read, that was wrong.
func TestUsageIndexListsEveryCommand(t *testing.T) {
	out := captureStdout(t, printUsage)
	for _, noun := range nouns {
		verbs := nounVerbs(noun)
		for _, verb := range verbs {
			if !strings.Contains(out, noun+" "+verb) {
				t.Errorf("irgo help does not mention %q on its front page", noun+" "+verb)
			}
		}
	}
}

// TestEveryDeclaredBuildTargetIsDispatched checks the list against the code
// that has to honour it.
//
// Comparing the list to the help would prove nothing — the help is rendered
// from the list, so the two agree by construction and the test passes for a
// target that does not exist. What can actually be wrong is the list promising
// a target runBuild has no case for, which fails at run time with "unknown
// target" after the help said it was supported.
func TestEveryDeclaredBuildTargetIsDispatched(t *testing.T) {
	// Two files and two shapes, because desktop is dispatched by an if in
	// runAppBuild while the rest are cases in runBuild. That inconsistency is
	// why this reads the source rather than the switch: there is no single
	// switch to read.
	var body string
	for _, f := range []string{"cmd_app_build.go", "cmd_app.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		body += string(src)
	}
	for _, target := range buildTargets() {
		// Either a target registered its own handler, or one of the two
		// dispatchers names it. Source text alone stopped being the whole
		// answer once targets could dispatch themselves — and a test that only
		// reads source would call a self-registering target undispatched.
		if _, ok := targetRuns["app build "+target]; ok {
			continue
		}
		if strings.Contains(body, `case "`+target+`"`) || strings.Contains(body, `== "`+target+`"`) {
			continue
		}
		t.Errorf("buildTargets promises %q but nothing dispatches it — "+
			"the help would advertise a target that fails at run time", target)
	}
}

// TestEveryVerbIsDeclared — the index, the detail page and the generated
// README all read commands.go, so a verb missing from it renders as three
// blanks and errors in none of them.
func TestEveryVerbIsDeclared(t *testing.T) {
	for _, noun := range nouns {
		verbs := nounVerbs(noun)
		for _, verb := range verbs {
			c, ok := commands[noun+" "+verb]
			if !ok {
				t.Errorf("%s %s is dispatched but has no entry in commands.go — "+
					"no help, no index line, no README row", noun, verb)
				continue
			}
			if c.summary == "" {
				t.Errorf("%s %s has no summary — it renders as a blank row", noun, verb)
			}
			if len(c.usage) == 0 {
				t.Errorf("%s %s has no usage lines", noun, verb)
			}
		}
	}
}

// TestEveryFlagIsDocumented finds flags nobody can discover.
//
// An audit once turned up nine, including every macOS and Windows signing
// override — the way CI supplies secrets without writing them to disk. Being
// undocumented made the documented path look like the only one.
func TestEveryFlagIsDocumented(t *testing.T) {
	// Flags irgo passes to other programs rather than accepts itself.
	passedThrough := map[string]bool{
		"--minify": true, // tailwind
		"--depth":  true, // git clone
		"--exists": true, // pkg-config
		"--yes":    true, // npx
		"--help":   true, // handled before dispatch
	}

	help := captureStdout(t, printUsage)
	for _, noun := range nouns {
		verbs := nounVerbs(noun)
		help += captureStdout(t, func() { printCommandHelp(noun, "") })
		for _, verb := range verbs {
			help += captureStdout(t, func() { printCommandHelp(noun, verb) })
		}
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	flag := regexp.MustCompile(`"(--[a-z][a-z-]+)"`)
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "cmd_") || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range flag.FindAllStringSubmatch(string(src), -1) {
			f := m[1]
			if passedThrough[f] || seen[f] {
				continue
			}
			seen[f] = true
			if !strings.Contains(help, f) {
				t.Errorf("%s accepts %s and no help mentions it", name, f)
			}
		}
	}
}

// TestDeclaredFlagsExist is the other direction: help.go can now promise a
// flag the CLI never reads, which is worse than an undocumented one — someone
// passes it, nothing happens, and nothing says so.
func TestDeclaredFlagsExist(t *testing.T) {
	var src string
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") && n != "commands.go" {
			b, err := os.ReadFile(n)
			if err != nil {
				t.Fatal(err)
			}
			src += string(b)
		}
	}
	name := regexp.MustCompile(`^(--[a-z][a-z-]*)`)
	for key, c := range commands {
		for _, f := range c.flags {
			m := name.FindStringSubmatch(f[0])
			if m == nil {
				t.Errorf("%s: flag spec %q does not start with a --flag", key, f[0])
				continue
			}
			if !strings.Contains(src, `"`+m[1]+`"`) {
				t.Errorf("%s documents %s but no code reads it — it would be accepted and ignored",
					key, m[1])
			}
		}
	}
}

// TestDeployBuildsThroughTheBuildPath guards a shortcut that reintroduced a
// fixed bug.
//
// deployCloudflare called buildCloudflare directly, one layer below the
// asset regeneration every build path does, so a deploy could ship a stale
// stylesheet — measured: `app build` refreshed it, `app deploy` did not.
// Deploying must be building plus uploading, and nothing else.
func TestDeployBuildsThroughTheBuildPath(t *testing.T) {
	src, err := os.ReadFile("app_cloudflare_deploy.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "buildCloudflare(") {
		t.Error("deploy calls buildCloudflare directly — that skips ensureAssets " +
			"and ships whatever happens to be on disk; call runBuild instead")
	}
	if !strings.Contains(body, `runBuild("cloudflare"`) {
		t.Error("deploy does not build through runBuild, so it does not do what `app build` does")
	}
}

// TestEveryInstalledToolIsPinned catches the bug that keeps recurring here.
//
// gomobile installed @latest started requiring golang.org/x/mobile in the
// project's dependency graph, and every mobile build broke with no commit in
// this repository or any project's. The x/mobile checkout had the same problem
// — cloned from the default branch with no ref. A build that changes without a
// commit is a build nobody can reproduce.
func TestEveryInstalledToolIsPinned(t *testing.T) {
	for _, tool := range []string{"templ", "air", "gomobile", "gobind"} {
		pkg := goToolPkg(tool)
		if pkg == "" {
			t.Errorf("%s has no install source", tool)
			continue
		}
		_, version, ok := strings.Cut(pkg, "@")
		if !ok {
			t.Errorf("%s installs from %q with no version at all", tool, pkg)
			continue
		}
		if version == "latest" {
			t.Errorf("%s installs @latest — pin it, or a build changes with no commit", tool)
		}
	}
}

// TestInstallRecordsThePinnedVersion — doctor reads what installation wrote,
// so the two have to agree. The first version of this asked the tool instead
// and reported air as drifted because its usage text contains a Go version.
func TestInstallRecordsThePinnedVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tool := range []string{"templ", "air", "gomobile", "gobind"} {
		markToolInstalled(tool)
		got := installedVersion(tool)
		if got == "" {
			t.Errorf("%s: installation recorded no version, so doctor cannot report one", tool)
			continue
		}
		if want := wantedVersion(tool); got != strings.TrimPrefix(want, "v") && got != want {
			t.Errorf("%s: recorded %q, irgo installs %q", tool, got, want)
		}
	}
}

// TestTestRegeneratesAssets — the help says `project test` regenerates before
// running, CI was written to trust that, and the code did not. On a fresh
// clone the templates package has no Go files and every test fails to build.
func TestTestRegeneratesAssets(t *testing.T) {
	src, err := os.ReadFile("cmd_server.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	i := strings.Index(body, "func runTest()")
	if i < 0 {
		t.Fatal("runTest not found")
	}
	if !strings.Contains(body[i:], "ensureAssets()") {
		t.Error("project test does not regenerate assets, but its help says it does — " +
			"a fresh clone cannot compile the templates package")
	}
}

// TestNoUnpinnedDatastarCDN — the client came from an unversioned CDN URL in
// one place and a release candidate in another. Neither should come back.
func TestNoUnpinnedDatastarCDN(t *testing.T) {
	roots := []string{"../../pkg", "../../cmd", "../../examples"}
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".templ", ".tmpl", ".html":
			default:
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if strings.Contains(string(body), "cdn.jsdelivr.net") &&
				strings.Contains(string(body), "datastar") {
				t.Errorf("%s loads Datastar from a CDN — it is embedded in pkg/datastarjs "+
					"and served at /_irgo/datastar.js, so the client and the Go library "+
					"cannot drift apart", path)
			}
			return nil
		})
	}
}

// TestEveryCommandHasARenderedNoun — a command registers itself from the file
// that implements it, so nothing checks its noun at the point of declaration.
//
// A noun outside `nouns` registers fine, dispatches fine, and appears in no
// help at all: the index iterates nouns, so the command exists and is
// undiscoverable. That is the cost of decentralising the table, and this is
// what pays it.
func TestEveryCommandHasARenderedNoun(t *testing.T) {
	known := map[string]bool{}
	for _, n := range nouns {
		known[n] = true
	}
	for key, c := range commands {
		if !known[c.noun] {
			t.Errorf("%q declares noun %q, which is not in `nouns` — it would "+
				"dispatch but never appear in the usage index or the README",
				key, c.noun)
		}
		if c.verb == "" {
			t.Errorf("%q registered with an empty verb", key)
		}
	}
}

// TestNounVerbsIsStable — ordering comes from the order field, not from the
// order init happened to run in, which is filename order and changes when a
// file is renamed.
func TestNounVerbsIsStable(t *testing.T) {
	for _, noun := range nouns {
		first := nounVerbs(noun)
		for i := 0; i < 5; i++ {
			again := nounVerbs(noun)
			if len(first) != len(again) {
				t.Fatalf("%s: unstable length", noun)
			}
			for j := range first {
				if first[j] != again[j] {
					t.Fatalf("%s: order changed between calls: %v vs %v", noun, first, again)
				}
			}
		}
	}
}

// TestTheCommandSetIsWhatWeThinkItIs — every command, listed once, so losing
// one is a test failure rather than a quiet absence.
//
// This is the cost of letting commands register themselves: a declaration that
// is dropped — by a bad rebase, a deleted init, a file moved without it — does
// not fail anything. The command simply is not there. nounVerbs is derived
// from what registered, so it agrees; the help is consistent; every other test
// passes. It happened while rebasing this very change, and `irgo project pin`
// vanished with nothing to say so.
//
// A list in a test is not the central table this replaced. It asserts rather
// than defines, and changing it is the deliberate act that adding or removing
// a command should be.
func TestTheCommandSetIsWhatWeThinkItIs(t *testing.T) {
	want := []string{
		"project new", "project clean", "project upgrade", "project pin",
		"project ci", "project assets", "project test", "project config",
		"app build", "app run", "app package", "app deploy",
		"app install", "app remove", "app reviews",
		"tools install", "tools remove", "tools doctor",
		"server dev", "server serve",
	}
	have := map[string]bool{}
	for k := range allCommands() {
		have[k] = true
	}
	for _, k := range want {
		if !have[k] {
			t.Errorf("%q is not registered — its init was lost, and nothing else "+
				"would have noticed", k)
		}
		delete(have, k)
	}
	for k := range have {
		t.Errorf("%q is registered but not in this list. If it is new, add it "+
			"here; that is the point of the list", k)
	}
}
