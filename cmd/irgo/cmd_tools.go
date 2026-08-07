// Toolchain: installing, verifying and removing the tools a build needs
// (templ, air, gomobile/gobind, mingw-w64) plus the generated assets they
// produce. Every install here has a matching uninstall.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runTempl generates templ files
func runTempl() error {
	if err := ensureGoTool("templ"); err != nil {
		return err
	}

	fmt.Println("Generating templ files...")
	return runCommand("templ", "generate")
}

// goToolPkg maps a tool name to the module to `go install`. templ is pinned to
// the project's go.mod version: the generator and the library must match, and
// @latest drift breaks generated code.
// pinTempl is the fallback templ version, used when a project's own cannot be
// read from its go.mod.
const pinTempl = "v0.3.977"

// pinAir is the hot-reload watcher's version. Pinned for the same reason as
// everything else irgo installs: a build that changes without a commit is a
// build nobody can reproduce.
const pinAir = "v1.63.0"

func goToolPkg(name string) string {
	switch name {
	case "templ":
		// The project's own templ version first: the generator and the runtime
		// package have to agree, or generated code fails to compile against
		// the library. Falling back to @latest made that disagreement possible
		// whenever the version could not be read.
		if v := templVersionFromGoMod(); v != "" {
			return "github.com/a-h/templ/cmd/templ@v" + v
		}
		return "github.com/a-h/templ/cmd/templ@" + pinTempl
	case "air":
		return "github.com/air-verse/air@" + pinAir
	case "gomobile", "gobind":
		// Same commit as the x/mobile checkout gomobile builds against.
		// Installing @latest while pinning the source is how a tool ends up
		// disagreeing with the code it drives: gomobile started requiring
		// golang.org/x/mobile in the project's dependency graph, and every
		// mobile build broke with no commit in this repository or the
		// project's.
		return "golang.org/x/mobile/cmd/" + name + "@" + pinXMobile
	}
	return ""
}

// irgoToolsDir holds one marker file per tool irgo installed itself, so
// uninstall can remove exactly those and leave a developer's own copies alone.
// Without it an uninstall would either delete tools irgo never owned or, if it
// played safe and skipped them, leave a half-provisioned machine that silently
// fails to re-provision.
func irgoToolsDir() string {
	return filepath.Join(homeDir(), ".irgo", "tools")
}

func markToolInstalled(name string) {
	dir := irgoToolsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	// Record which version was installed, not just that one was. Asking the
	// tool afterwards does not work: air and gobind have no version flag at
	// all, and parsing their usage text finds the Go version instead, which is
	// how doctor first reported air as drifted when it was not.
	body := "irgo " + version + "\n"
	if pkg := goToolPkg(name); pkg != "" {
		if _, v, ok := strings.Cut(pkg, "@"); ok {
			body += "pin " + v + "\n"
		}
	}
	_ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
}

// clearToolMarker forgets that irgo installed something. Markers left behind
// keep ~/.irgo populated, which makes an otherwise-clean machine look
// provisioned — and a machine that cannot reach a known-clean state hides
// provisioning bugs.
func clearToolMarker(name string) {
	_ = os.Remove(filepath.Join(irgoToolsDir(), name))
}

func toolInstalledByIrgo(name string) bool {
	_, err := os.Stat(filepath.Join(irgoToolsDir(), name))
	return err == nil
}

// ensureGoTool installs a Go-based tool when missing rather than printing an
// install command for someone to copy. `go install` behaves the same on macOS,
// Linux and Windows, so this needs no per-OS branching.
func ensureGoTool(name string) error {
	if _, err := exec.LookPath(name); err == nil {
		return nil
	}
	pkg := goToolPkg(name)
	if pkg == "" {
		return fmt.Errorf("%s not found, and no install source is known for it", name)
	}
	fmt.Printf("%s not found — installing %s...\n", name, pkg)
	if err := runCommand(goBin(), "install", pkg); err != nil {
		return fmt.Errorf("installing %s: %w", name, err)
	}
	markToolInstalled(name)
	// `go install` lands in GOBIN (default $GOPATH/bin), which is often absent
	// from PATH — prepend it so the tool resolves for the rest of this process.
	if dir := gobinDir(); dir != "" {
		_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s still not found after installing it — is %s on your PATH?", name, gobinDir())
	}
	return nil
}

// runCSS rebuilds the Tailwind stylesheet. static/css/output.css is generated
// and gitignored, but it is embedded into every build — so skipping it ships an
// unstyled app. No-op for projects without a Tailwind entry point.
func runCSS() error {
	const in, out = "static/css/input.css", "static/css/output.css"
	if _, err := os.Stat(in); err != nil {
		return nil // not a Tailwind project
	}
	bin, err := ensureTailwind()
	if err != nil {
		return err
	}
	fmt.Println("Building CSS...")
	return runCommand(bin, "-i", in, "-o", out, "--minify")
}

// ensureAssets regenerates everything that is gitignored yet embedded into a
// build: _templ.go and the Tailwind stylesheet. Every build path runs this, so
// a fresh clone builds correctly without the caller sequencing it by hand.
func ensureAssets() error {
	if err := runTempl(); err != nil {
		return err
	}
	return runCSS()
}

// installTools installs required development tools
func installTools() error {
	fmt.Println("Installing irgo development tools...")
	fmt.Println()

	// ensureGoTool owns pinning (templ tracks the project's go.mod version),
	// marker-writing for uninstall, and PATH fix-up. This is just the eager
	// form of what every build does lazily.
	for _, tool := range []string{"templ", "air", "gomobile"} {
		if _, err := exec.LookPath(tool); err == nil {
			fmt.Printf("  %s: already installed\n", tool)
			continue
		}
		if err := ensureGoTool(tool); err != nil {
			fmt.Printf("  Warning: %v\n", err)
			continue
		}
		fmt.Printf("  %s: installed\n", tool)
	}

	if err := installOSPackages(); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Initializing gomobile...")
	if err := runCommand("gomobile", "init"); err != nil {
		fmt.Printf("Warning: gomobile init failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Done. Nothing else is required up front:")
	fmt.Println("  Android — irgo installs the JDK, SDK and NDK on the first")
	fmt.Println("            android build or run. Android Studio is NOT needed.")
	fmt.Println("            Check it with: irgo tools doctor android")
	if runtime.GOOS == "darwin" {
		fmt.Println("  iOS     — needs Xcode from the App Store (the one thing irgo")
		fmt.Println("            cannot install for you).")
		fmt.Println("  Windows — irgo installs mingw-w64 when cross-compiling.")
	} else {
		fmt.Printf("  iOS     — deploys to a physical device from %s via xtool +\n", runtime.GOOS)
		fmt.Println("            a Swift toolchain (https://xtool.sh); run 'xtool setup'")
		fmt.Println("            once. The Simulator itself still needs macOS.")
	}
	return nil
}

// uninstallTools is the exact inverse of `irgo tools install`: it removes the
// Go tools irgo installed, and nothing else. Every install path in the CLI has
// a matching uninstall — without one you cannot return a machine to a known
// state, and a provisioning bug hides behind whatever was left lying around
// instead of surfacing on the next run.
//
// Marker-guarded: a tool irgo did not install is reported and kept, so a
// developer's own templ/air survives. Pass all to override that.
// removalTally counts outcomes so the summary reflects everything considered,
// not just the Go tools.
type removalTally struct{ removed, kept, missing int }

// report prints one aligned line and records the outcome. Aligned columns
// because the previous output interleaved three kinds of thing in one flat
// list, and you could not tell a Go tool from a Homebrew package.
func (t *removalTally) report(name, state, detail string) {
	switch state {
	case "removed":
		t.removed++
	case "kept":
		t.kept++
	default:
		t.missing++
	}
	if detail != "" {
		fmt.Printf("  %-14s %-9s %s\n", name, state, detail)
		return
	}
	fmt.Printf("  %-14s %s\n", name, state)
}

// uninstallTools is the exact inverse of `irgo tools install`: it removes what
// irgo installed, and nothing else. Every install path in the CLI has a
// matching removal — without one you cannot return a machine to a known state,
// and a provisioning bug hides behind whatever was left lying around instead of
// surfacing on the next run.
//
// Marker-guarded: a tool irgo did not install is reported and kept, so a
// developer's own templ or gomobile survives. Pass all to override that.
// uninstallTools removes what irgo installed — all of it, including Android.
//
// scope narrows it to one area ("android"), yes skips the prompt, all also
// removes copies irgo did not install, and keepJDK preserves the managed JDK,
// which is the slowest thing here to re-download.
func uninstallTools(scope string, all, yes, keepJDK bool) error {
	var p removalPlan
	switch scope {
	case "android":
		planAndroid(&p, keepJDK)
	default:
		planGoTools(&p, all)
		planDownloads(&p)
		planNode(&p)
		planHostPackages(&p, all)
		planAndroid(&p, keepJDK)
	}

	if p.empty() {
		fmt.Println("Nothing to remove — irgo has not installed anything here.")
		if len(p.kept) > 0 {
			fmt.Printf("Present but not installed by irgo: %s\n", strings.Join(p.kept, ", "))
			fmt.Println("  --all removes them too.")
		}
		return nil
	}

	ok, err := confirmRemoval(&p, yes)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	fmt.Println("Removing:")
	var t removalTally
	for _, act := range p.acts {
		act(&t)
	}

	// Build residue from mobile builds: the temp x/mobile clone and the local
	// go.work that ensureMobileBuildSetup generates.
	_ = os.RemoveAll(filepath.Join(os.TempDir(), "golang-mobile"))
	_ = os.Remove("go.work")
	_ = os.Remove("go.work.sum")
	pruneIrgoStateDir()

	fmt.Printf("\n%d removed, %d kept, %d absent.\n", t.removed, t.kept, t.missing)
	return nil
}

func checkTool(name, installCmd string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found. Install with: %s", name, installCmd)
	}
	return nil
}

// pruneIrgoStateDir removes ~/.irgo once nothing is left in it. Marker files
// are irgo's own bookkeeping, so leaving an empty tree behind makes an
// otherwise-clean machine look provisioned.
func pruneIrgoStateDir() {
	// os.Remove only succeeds on an empty directory, which is exactly the
	// condition we want — never delete state that is still in use.
	_ = os.Remove(irgoToolsDir())
	_ = os.Remove(filepath.Join(homeDir(), ".irgo"))
}
