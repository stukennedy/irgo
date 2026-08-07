// Shared mobile build plumbing: target dispatch, host gating, the gomobile
// workspace, and the artifact stamps that force a rebuild after an upgrade.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// runBuild builds for mobile platforms. sim additionally builds the runnable
// iOS Simulator app (ios target only).
func runBuild(target string, sim, device bool, team string) error {
	// No gomobile check here: buildIOS/buildAndroid both run
	// ensureMobileBuildSetup, which installs gomobile + gobind when missing.
	// Gating up front would fail the build before that could happen.

	// Determine module path
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}

	// _templ.go and output.css are generated and gitignored, yet the mobile
	// package imports the former and every build embeds the latter. Regenerate
	// both here rather than making callers remember the ordering.
	if err := ensureAssets(); err != nil {
		return err
	}

	// Create build directory
	if err := os.MkdirAll("build", 0755); err != nil {
		return fmt.Errorf("creating build directory: %w", err)
	}

	if sim && device {
		return fmt.Errorf("--sim and --device build different things: " +
			"--sim is an unsigned Simulator app, --device is a signed build for real hardware. Pick one")
	}

	switch target {
	case "ios":
		// The Simulator and Xcode-signed device apps genuinely need Xcode;
		// the plain framework build works on Linux too, via the xtool
		// Darwin SDK (buildIOS dispatches — see app_ios_xtool.go).
		if sim || device {
			if err := requireMacOS("iOS app"); err != nil {
				return err
			}
			return buildIOSApp(modulePath, device, team)
		}
		return buildIOS(modulePath)
	case "android":
		return buildAndroid(modulePath)
	case "cloudflare":
		return buildCloudflare(modulePath)
	case "all":
		// Android builds anywhere; iOS needs Xcode (macOS) or the xtool
		// Darwin SDK (Linux). Skip rather than fail when neither is there,
		// so `irgo app build all` stays usable on Linux/Windows CI.
		switch {
		case runtime.GOOS == "darwin":
			if err := buildIOS(modulePath); err != nil {
				return err
			}
		default:
			if _, err := findAppleToolchain(); err == nil {
				if err := buildIOS(modulePath); err != nil {
					return err
				}
			} else {
				fmt.Printf("Skipping iOS framework: needs macOS (Xcode) or the xtool Darwin SDK (host is %s)\n", runtime.GOOS)
			}
		}
		return buildAndroid(modulePath)
	default:
		return fmt.Errorf("unknown build target: %s (use ios, android, or all)", target)
	}
}

// requireMacOS gates Apple-only targets with an actionable error, instead of
// letting the build fail later on a missing xcodebuild that the host can never
// have in the first place.
func requireMacOS(what string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s builds require macOS (Xcode) — cannot cross-compile from %s.\n  Run `irgo tools doctor` to see everything this host can and cannot build", what, runtime.GOOS)
	}
	return nil
}

// runMobile builds and runs on mobile simulator
func runMobile(platform string, devMode bool) error {
	// Same as the build paths: _templ.go and output.css are generated and
	// gitignored but compiled into the app, so running without regenerating
	// them ships whatever happened to be on disk — or, after irgo project clean,
	// nothing at all, which looks like a broken app rather than a missing step.
	if err := ensureAssets(); err != nil {
		return err
	}
	switch platform {
	case "ios":
		return runIOS(devMode)
	case "android":
		return runAndroid(devMode)
	default:
		return fmt.Errorf("unknown platform: %s (use ios or android)", platform)
	}
}

// ensureMobileBuildSetup ensures the go.work file and x/mobile are set up correctly
// pinXMobile is the golang/mobile commit gomobile is built from. Bump it
// deliberately; leaving it to upstream's default branch means a build that
// worked yesterday can fail today for reasons in neither repository.
const pinXMobile = "62cee1672c8eb8502c4718ec93e6ae321d1e40e5"

// ensureGobindTool puts golang.org/x/mobile in the project's dependency graph.
//
// gomobile bind refuses to run without it: "gomobile bind requires
// golang.org/x/mobile in the current module, but it is not in the module
// dependency graph". Nothing in a generated project imports x/mobile — the
// bound package imports irgo, and irgo imports nothing from x/mobile — so
// `go mod tidy` will never add it and every project would meet this error on
// its first mobile build.
func ensureGobindTool() error {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return nil // not a project root; the build will say so
	}
	if strings.Contains(string(data), "golang.org/x/mobile") {
		return nil
	}
	fmt.Println("Adding golang.org/x/mobile (gomobile bind requires it)...")
	cmd := exec.Command(goBin(), "get", "-tool", "golang.org/x/mobile/cmd/gobind")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, gerr := cmd.CombinedOutput(); gerr != nil {
		return fmt.Errorf("adding the gobind tool: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureMobileBuildSetup() error {
	if err := ensureGobindTool(); err != nil {
		return err
	}
	// The workspace and the vendored x/mobile follow this machine's toolchain,
	// not the floor a generated project declares.
	goVersion := runningGoVersion()

	// Check if go.work exists with x/mobile
	if _, err := os.Stat("go.work"); os.IsNotExist(err) {
		// Get irgo path for replacement
		irgoPath := getIrgoPath()

		// Clone x/mobile if not already present
		mobileDir := filepath.Join(os.TempDir(), "golang-mobile")
		if _, err := os.Stat(mobileDir); os.IsNotExist(err) {
			fmt.Println("Cloning golang.org/x/mobile...")
			// Pinned to a commit. `git clone --depth 1` with no ref tracks
			// whatever upstream main is that morning, so a mobile build could
			// break with no commit in this repository or the project's — the
			// same unpinned-input bug as installing a tool @latest. golang/mobile
			// publishes no tags, so the pin is a SHA fetched directly.
			if err := os.MkdirAll(mobileDir, 0o755); err != nil {
				return err
			}
			for _, step := range [][]string{
				{"init", "-q"},
				{"remote", "add", "origin", "https://github.com/golang/mobile"},
				{"fetch", "-q", "--depth", "1", "origin", pinXMobile},
				{"checkout", "-q", "FETCH_HEAD"},
			} {
				cmd := exec.Command("git", step...)
				cmd.Dir = mobileDir
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					os.RemoveAll(mobileDir)
					return fmt.Errorf("fetching x/mobile %s: %w", pinXMobile[:12], err)
				}
			}
		}

		// Update go.mod in cloned repo to use current Go version
		mobileModPath := filepath.Join(mobileDir, "go.mod")
		if data, err := os.ReadFile(mobileModPath); err == nil {
			content := string(data)
			// Replace any go 1.x.x version with current version
			lines := splitLines(content)
			for i, line := range lines {
				if len(line) > 3 && line[:3] == "go " {
					lines[i] = "go " + goVersion
					break
				}
			}
			os.WriteFile(mobileModPath, []byte(strings.Join(lines, "\n")), 0644)
		}

		// Create go.work file
		workContent := fmt.Sprintf("go %s\n\nuse (\n\t.\n", goVersion)
		// Not when it is this directory: building irgo's own mobile package
		// from the irgo checkout put the same module in the workspace twice,
		// and Go rejects the whole file — "appears multiple times in
		// workspace" — so every mobile build in the framework repository
		// failed until the file was deleted by hand.
		if irgoPath != "" && !sameDir(irgoPath, ".") {
			workContent += fmt.Sprintf("\t%s\n", irgoPath)
		}
		workContent += fmt.Sprintf("\t%s\n)\n", mobileDir)

		if err := os.WriteFile("go.work", []byte(workContent), 0644); err != nil {
			return fmt.Errorf("failed to create go.work: %w", err)
		}
		fmt.Println("Created go.work for mobile build")

		// Install gomobile and gobind from local source
		fmt.Println("Installing gomobile from source...")
		cmd := exec.Command(goBin(), "install", "./cmd/gomobile")
		cmd.Dir = mobileDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install gomobile: %w", err)
		}

		cmd = exec.Command(goBin(), "install", "./cmd/gobind")
		cmd.Dir = mobileDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install gobind: %w", err)
		}

		// `go install` lands in GOBIN (default $GOPATH/bin), which may not be on
		// the caller's PATH — prepend it so runGomobileCommand resolves gomobile.
		_ = os.Setenv("PATH", gobinDir()+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	return nil
}

// runGomobileCommand runs a gomobile command with the correct GOTOOLCHAIN
func runGomobileCommand(args ...string) error {
	// The toolchain this machine runs, not the floor a generated project
	// declares. Pinning GOTOOLCHAIN below what go.work requires makes Go
	// refuse the workspace, and gomobile reports that as "missing
	// golang.org/x/mobile dependency" — which is true of neither the module
	// nor the workspace, and sends you looking in the wrong place entirely.
	goVersion := runningGoVersion()

	// gomobile bind for Android shells out to javac, which needs a JDK on
	// PATH — resolve the managed ~/.irgo/jdks JDK (or an existing one) so the
	// toolchain is self-contained after `irgo tools install android`.
	applyBestJDKToEnv()

	cmd := exec.Command("gomobile", args...)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go"+goVersion)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// writeArtifactStamp records which irgo produced a build directory.
//
// Every build output carries one. Two of them are load-bearing: the gomobile
// AAR and xcframework are expensive to rebuild and must match the bridge API
// of the irgo that generated them, so a stamp from another version forces a
// rebuild rather than failing to compile against the new one.
//
// The rest are for the question that arises with anything deployed or shipped:
// which version built this? A worker uploaded to Cloudflare or an app handed
// to someone is otherwise anonymous, and it used to be that only some builds
// answered.
func writeArtifactStamp(dir string) {
	_ = os.WriteFile(filepath.Join(dir, ".irgo-version"), []byte(version), 0644)
}

func artifactUpToDate(artifact, stampDir string) bool {
	fi, err := os.Stat(artifact)
	if err != nil {
		return false
	}
	data, rerr := os.ReadFile(filepath.Join(stampDir, ".irgo-version"))
	if rerr != nil || strings.TrimSpace(string(data)) != version {
		return false
	}
	// The framework embeds the app's Go code and static assets, so a source
	// edit makes it stale even when the CLI version is unchanged. Without this
	// dev mode reuses a framework built before the change and the app runs
	// old code with no indication why.
	return !anySourceNewerThan(fi.ModTime())
}

// anySourceNewerThan reports whether any embedded source is newer than t.
func anySourceNewerThan(t time.Time) bool {
	for _, dir := range []string{"static", "templates", "handlers", "app", "mobile"} {
		newer := false
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, ierr := d.Info(); ierr == nil && info.ModTime().After(t) {
				newer = true
				return filepath.SkipAll
			}
			return nil
		})
		if newer {
			return true
		}
	}
	// main.go and go.mod sit at the root rather than in a package directory.
	for _, f := range []string{"main.go", "go.mod"} {
		if info, err := os.Stat(f); err == nil && info.ModTime().After(t) {
			return true
		}
	}
	return false
}

// sameDir reports whether two paths are the same directory.
func sameDir(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	if aa == bb {
		return true
	}
	// Symlinks: /tmp and /private/tmp are the same place on macOS.
	ra, err1 := filepath.EvalSymlinks(aa)
	rb, err2 := filepath.EvalSymlinks(bb)
	return err1 == nil && err2 == nil && ra == rb
}
