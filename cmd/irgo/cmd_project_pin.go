// Choosing which irgo a project builds against.
//
// There is no separate class of person here. Using irgo and working on irgo are
// the same activity pointed at different versions: a published release, a fork
// tag, or a checkout on your disk. `go tool irgo` reads whichever go.mod names,
// so one command covers all three and nothing else in the project changes.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const upstreamModule = "github.com/stukennedy/irgo"

// runPin reports or changes the pinned CLI.
func runPin(args []string) error {
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("no go.mod here — run irgo project pin from your project root")
	}
	if len(args) == 0 {
		return showPin()
	}

	// Bare words, like every other command: nothing in the CLI takes a flag
	// where a word will do.
	target := args[0]
	switch {
	case target == "local":
		dir := "."
		if len(args) > 1 {
			dir = args[1]
		}
		return pinLocal(dir)
	case target == "release":
		return pinRelease()
	default:
		return pinVersion(target)
	}
}

// looksLikeVersion reports whether target is a version rather than a word
// someone meant as a keyword. Without this check a typo is silently turned
// into a tag ("local" -> "vlocal") that no repository has, and the pin only
// fails later in `go mod tidy` — after go.mod has already been rewritten.
func looksLikeVersion(target string) bool {
	v := strings.TrimPrefix(target, "v")
	return v != "" && v[0] >= '0' && v[0] <= '9'
}

// showPin prints what the project builds against and where that came from.
func showPin() error {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	var require, replace string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "replace "+upstreamModule) {
			if i := strings.Index(line, "=>"); i >= 0 {
				replace = strings.TrimSpace(line[i+2:])
			}
		}
		if strings.HasPrefix(line, upstreamModule+" v") || strings.HasPrefix(line, "require "+upstreamModule) {
			require = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		}
	}

	fmt.Println("irgo project pin (from go.mod)")
	fmt.Println()
	if replace == "" {
		fmt.Printf("  published release: %s\n", strings.TrimSpace(require))
		fmt.Println("  `go tool irgo` builds that version from the module proxy.")
	} else if strings.HasPrefix(replace, ".") || strings.HasPrefix(replace, "/") {
		fmt.Printf("  local checkout: %s\n", replace)
		fmt.Println("  `go tool irgo` builds your working tree — edit the CLI and")
		fmt.Println("  the next command already reflects it. No install step.")
	} else {
		fmt.Printf("  fork: %s\n", replace)
		fmt.Println("  `go tool irgo` clones and builds that tag on demand.")
	}

	fmt.Println()
	fmt.Println("Change it:")
	fmt.Println("  irgo project pin release              track the published module")
	fmt.Println("  irgo project pin <version>            a published version")
	fmt.Println("  irgo project pin <owner>/<repo>@<tag> track a fork")
	fmt.Println("  irgo project pin local [dir]          build a checkout you are editing")
	return nil
}

// pinLocal points the project at a working tree, which is how you work ON the
// CLI: edit, run, no install, no tagging, no reinstall.
func pinLocal(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return fmt.Errorf("%s is not a Go module — point it at an irgo checkout", abs)
	}
	data, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "module "+upstreamModule) {
		return fmt.Errorf("%s is a Go module but not irgo (its go.mod declares a different module)", abs)
	}

	restore, err := snapshotGoMod()
	if err != nil {
		return err
	}
	if err := goModEdit("-replace", upstreamModule+"="+abs); err != nil {
		return err
	}
	if err := tidy(); err != nil {
		restore()
		return err
	}
	fmt.Printf("Pinned to your checkout: %s\n", abs)
	fmt.Println("`go tool irgo` now builds that tree — edits take effect immediately.")
	return nil
}

// pinRelease drops any replace, returning to the published module.
func pinRelease() error {
	restore, err := snapshotGoMod()
	if err != nil {
		return err
	}
	if err := goModEdit("-dropreplace", upstreamModule); err != nil {
		return err
	}
	if err := tidy(); err != nil {
		restore()
		return err
	}
	fmt.Println("Pinned to the published module (replace removed).")
	fmt.Println("Change the version with: irgo project pin <version>")
	return nil
}

// pinVersion accepts a bare version for the published module, or
// owner/repo@version for a fork.
func pinVersion(target string) error {
	if strings.Contains(target, "/") {
		repo, version, ok := strings.Cut(target, "@")
		if !ok {
			return fmt.Errorf("a fork needs a version: irgo project pin %s@<tag>", target)
		}
		mod := repo
		if !strings.HasPrefix(mod, "github.com/") {
			mod = "github.com/" + mod
		}
		restore, err := snapshotGoMod()
		if err != nil {
			return err
		}
		if err := goModEdit("-replace", upstreamModule+"="+mod+"@"+version); err != nil {
			return err
		}
		if err := tidy(); err != nil {
			restore()
			return err
		}
		fmt.Printf("Pinned to fork %s %s\n", mod, version)
		// A fork keeps the upstream module path, so the proxy cannot serve it.
		fmt.Println()
		fmt.Println("A fork is fetched straight from GitHub. If this is the first one,")
		fmt.Printf("tell Go not to use the proxy for it:\n  go env -w GOPRIVATE='%s/*'\n",
			strings.TrimSuffix(mod, "/"+filepath.Base(mod)))
		return nil
	}

	if !looksLikeVersion(target) {
		return fmt.Errorf("%q is not a version. Did you mean one of:\n"+
			"  irgo project pin local [dir]     a checkout on this machine\n"+
			"  irgo project pin release         the latest published release\n"+
			"  irgo project pin v0.4.0          a published version\n"+
			"  irgo project pin owner/irgo@tag  a fork", target)
	}
	if !strings.HasPrefix(target, "v") {
		target = "v" + target
	}
	// Pinning rewrites go.mod before Go gets a chance to reject the version, so
	// a bad tag would otherwise leave the project unbuildable and the person
	// holding a broken go.mod they never edited.
	restore, err := snapshotGoMod()
	if err != nil {
		return err
	}
	if err := goModEdit("-dropreplace", upstreamModule); err != nil {
		return err
	}
	if err := goModEdit("-require", upstreamModule+"@"+target); err != nil {
		restore()
		return err
	}
	if err := tidy(); err != nil {
		restore()
		return fmt.Errorf("%w\n\ngo.mod left unchanged — %s was not pinned", err, target)
	}
	fmt.Printf("Pinned to published %s\n", target)
	return nil
}

// snapshotGoMod returns a function that puts go.mod (and go.sum) back as they
// are right now.
func snapshotGoMod() (func(), error) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		return nil, err
	}
	sum, sumErr := os.ReadFile("go.sum")
	return func() {
		os.WriteFile("go.mod", mod, 0644)
		if sumErr == nil {
			os.WriteFile("go.sum", sum, 0644)
		}
	}, nil
}

// goCommand runs the Go toolchain irgo itself was built with.
func goCommand(args ...string) *exec.Cmd { return exec.Command(goBin(), args...) }

func goModEdit(args ...string) error {
	full := append([]string{"mod", "edit"}, args...)
	cmd := goCommand(full...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod edit failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// tidy resolves the new pin. -mod=mod because a tool directive plus a changed
// replace needs go.sum updated, which the default readonly mode refuses.
func tidy() error {
	cmd := goCommand("mod", "tidy")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		// A failing tidy means the pin does not resolve, which used to be
		// reported as a warning while go.mod kept the unbuildable edit.
		// Callers restore the previous go.mod, so this has to be an error.
		return fmt.Errorf("go mod tidy: %s\n"+
			"If this is a fork, Go may need: go env -w GOPRIVATE='github.com/<owner>/*'",
			strings.TrimSpace(string(out)))
	}
	fmt.Println("go.mod updated — `go tool irgo` now uses it.")
	return nil
}

// projectReplacement returns the replacement this project pins irgo to, or ""
// when it tracks the published module (or there is no go.mod here).
func projectReplacement() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "replace "+upstreamModule) {
			continue
		}
		if i := strings.Index(line, "=>"); i >= 0 {
			return strings.TrimSpace(line[i+2:])
		}
	}
	return ""
}

func init() {
	register(command{
		noun: "project", verb: "pin", order: 30,
		summary: "Choose which irgo this project builds against",
		args:    "[local|release|<version>]",
		usage: [][2]string{
			{"", "Show the current pin and where it came from"},
			{"local [dir]", "Build a checkout you are editing"},
			{"release", "Track the published module"},
			{"<version>", "A published version, e.g. v0.4.0"},
			{"<owner>/<repo>@<tag>", "A fork"},
		},
		notes: "go.mod is the only pin: `go tool irgo` builds whatever it names, so there is\n" +
			`nothing installed globally to fall out of step. A pin that does not resolve
leaves go.mod untouched rather than half-written.

A fork keeps the upstream module path, so the proxy cannot serve it:
  go env -w GOPRIVATE='github.com/<owner>/*'`,
	})
}
