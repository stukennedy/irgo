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

	"golang.org/x/mod/modfile"
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
		if err := requireMacOS("iOS"); err != nil {
			return err
		}
		if sim || device {
			return buildIOSApp(modulePath, device, team)
		}
		return buildIOS(modulePath)
	case "android":
		return buildAndroid(modulePath)
	case "cloudflare":
		return buildCloudflare(modulePath)
	case "all":
		// Android builds anywhere; iOS cannot leave macOS. Skip rather than
		// fail so `irgo app build all` stays usable on Linux/Windows CI.
		if runtime.GOOS == "darwin" {
			if err := buildIOS(modulePath); err != nil {
				return err
			}
		} else {
			fmt.Printf("Skipping iOS framework: requires macOS (host is %s)\n", runtime.GOOS)
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
	// An existing go.work can be stale: it references the temp x/mobile clone
	// (os.TempDir()/golang-mobile) that macOS may clean up between sessions. A
	// stale go.work breaks every `go` command with "use ...: directory does not
	// exist", and irgo previously never validated it — users had to delete it
	// by hand. Validate now; if any referenced dir is gone, drop the file (and
	// go.work.sum) so it is regenerated below.
	//
	// This must run before ensureGobindTool: its `go get` loads the workspace
	// and dies on the stale file, so validating afterwards would never repair
	// exactly the legacy projects this exists for.
	if _, err := os.Stat("go.work"); err == nil && !goWorkFileValid("go.work") {
		// Only files irgo itself generated are regenerated wholesale.
		// go.work is gitignored in generated projects, so deleting a
		// developer's customized workspace (replace directives, extra use
		// entries) would destroy configuration with no way to recover it.
		if !goWorkIrgoGenerated("go.work") {
			return fmt.Errorf("go.work is stale or unparseable, but looks customized (replace/extra use entries, or edits irgo cannot verify) so irgo will not overwrite it.\n" +
				"  Fix go.work by hand, or delete go.work and go.work.sum to let irgo regenerate them")
		}
		fmt.Println("go.work references missing directories — regenerating")
		os.Remove("go.work")
		os.Remove("go.work.sum")
	}

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

		// Clone x/mobile if not already present. "Present" means a usable
		// module: a partial clone (process killed between MkdirAll and
		// checkout) must be removed and re-cloned, or the regenerated
		// go.work would point at a directory Go still cannot load.
		mobileDir := filepath.Join(os.TempDir(), "golang-mobile")
		if !isModuleDir(mobileDir) {
			if _, statErr := os.Stat(mobileDir); statErr == nil {
				fmt.Println("Removing partial x/mobile clone...")
				os.RemoveAll(mobileDir)
			}
		}
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
		workContent := fmt.Sprintf("%s\n\ngo %s\n\nuse (\n\t.\n", goWorkGeneratedMarker, goVersion)
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

// goWorkFileValid reports whether every directory referenced by a go.work
// file still exists. A stale file (e.g. a cleaned-up temp x/mobile clone) is
// invalid so ensureMobileBuildSetup can regenerate it.
//
// Uses Go's own work-file parser: hand-splitting lines would misread inline
// comments (`. // root module`), quoted paths, and single-line `use`
// directives as missing directories — and deleting a user's customized
// workspace over a parse artifact is exactly the kind of damage this check
// exists to prevent. An unparseable file is treated as invalid: `go` would
// reject it anyway, so regenerating is the recovery.
func goWorkFileValid(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	wf, err := modfile.ParseWork(path, data, nil)
	if err != nil {
		return false
	}
	dir := filepath.Dir(path)
	for _, use := range wf.Use {
		p := use.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		// A use target must be a module, not merely an existing directory
		// (`go work use` semantics). A bare existence check would pass a
		// partial x/mobile clone — e.g. a process killed between MkdirAll
		// and checkout — and the workspace would stay broken. Any stat
		// error counts: ENOTDIR (path replaced by a file) and EACCES leave
		// the module just as unloadable as ENOENT, and go.mod must be a
		// regular file.
		if !isModuleDir(p) {
			return false
		}
	}
	return true
}

// goWorkIrgoGenerated reports whether a go.work file contains only the
// members irgo itself writes — the project root, the irgo checkout (local
// development), and the temp x/mobile clone — and no replace directives.
// Only such files are safe to delete and regenerate; anything else carries
// developer customization that must not be destroyed (go.work is gitignored
// in generated projects, so it is unrecoverable).
//
// An unparseable file counts as irgo-generated: Go rejects it outright, so
// there is no working configuration to preserve.
// goWorkGeneratedMarker is written into every go.work irgo creates. Its
// presence is definitive provenance: the file may be deleted and regenerated
// without heuristics. Editing the file by hand and keeping the marker opts
// into that behavior.
const goWorkGeneratedMarker = "// Code generated by irgo. Safe to delete; it will be regenerated on the next mobile build."

func goWorkIrgoGenerated(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Definitive provenance first; the shape heuristics below exist only for
	// legacy files generated before the marker was introduced.
	if strings.Contains(string(data), goWorkGeneratedMarker) {
		return true
	}
	// Without the marker, a parse failure carries no provenance — it may be
	// a developer's unfinished edit of a customized workspace, and deleting
	// it would lose whatever they were writing. Protect it; the error path
	// tells them how to recover.
	wf, err := modfile.ParseWork(path, data, nil)
	if err != nil {
		return false
	}
	// toolchain and godebug are workspace settings irgo never writes —
	// their presence means a developer configured this file by hand.
	if len(wf.Replace) > 0 || wf.Toolchain != nil || len(wf.Godebug) > 0 {
		return false
	}

	known := map[string]bool{
		filepath.Clean("."):                          true,
		filepath.Join(os.TempDir(), "golang-mobile"): true,
	}
	if irgoPath := getIrgoPath(); irgoPath != "" {
		known[filepath.Clean(irgoPath)] = true
	}

	dir := filepath.Dir(path)
	for _, use := range wf.Use {
		p := filepath.Clean(use.Path)
		abs := p
		if !filepath.IsAbs(abs) {
			// Relative entries resolve against the go.work directory.
			abs = filepath.Join(dir, abs)
		}
		if known[p] || known[abs] {
			continue
		}
		// Legacy files only (no marker): a go.work generated under an
		// earlier TMPDIR references that session's golang-mobile clone, not
		// the current os.TempDir()'s — exactly the macOS scenario this
		// repair exists for. Recognize such entries as irgo's own, but only
		// when the directory is gone: a live directory named golang-mobile
		// could be a real custom module. New files carry the marker, so
		// this heuristic ages out with pre-marker projects.
		if filepath.Base(p) == "golang-mobile" {
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				continue
			}
		}
		// Legacy files only: the entry may be an irgo checkout recorded by
		// a previous CLI whose getIrgoPath() no longer discovers it (e.g.
		// after switching from a locally built CLI to an installed one).
		// Its go.mod module declaration is the provenance — the same test
		// getIrgoPath itself uses.
		if isIrgoCheckout(abs) {
			continue
		}
		return false
	}
	return true
}

// isModuleDir reports whether dir contains a loadable module: a readable
// go.mod declaring a valid module path. Reading covers every unusable shape
// in one test — missing file (ENOENT), dir-in-place-of-file (EISDIR), path
// replaced by a file (ENOTDIR), unreadable (EACCES) — and parsing catches a
// truncated or malformed go.mod left by an interrupted checkout, which Go
// would reject just the same.
func isModuleDir(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	// Full strict parse: modfile.ModulePath tolerates errors elsewhere in
	// the file by contract, but Go itself does not — a valid module line
	// above a truncated require block still fails to load.
	f, err := modfile.Parse("go.mod", data, nil)
	return err == nil && f.Module != nil && f.Module.Mod.Path != ""
}

// isIrgoCheckout reports whether dir is a checkout of the irgo framework
// itself, identified by its go.mod module declaration. The path is parsed
// and matched exactly — a substring test would also accept e.g.
// github.com/stukennedy/irgo-tools, or the path appearing in a comment,
// and this result gates a destructive regeneration.
func isIrgoCheckout(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	return err == nil && modfile.ModulePath(data) == "github.com/stukennedy/irgo"
}
