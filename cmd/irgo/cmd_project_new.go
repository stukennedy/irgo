package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Use the all: prefix so files starting with "." or "_" (e.g. .gitignore,
// .air.toml, files under hidden directories) are embedded too - plain
// go:embed patterns skip them inside matched directories.
//
//go:embed all:templates
var templateFS embed.FS

// Datastar is served by the framework at /_irgo/datastar.js, embedded in
// pkg/datastarjs and versioned with irgo — see that package for why. Projects
// used to download a copy from a CDN here, which made `project new` need the
// network and left every project pinned to whatever client was current the day
// it was created.

// getGoVersion returns the current Go version (e.g., "1.24.12")
// minGoVersion is the Go a generated project requires.
//
// Deliberately a constant rather than whatever is installed here. Writing the
// developer's toolchain into go.mod pins every teammate and every CI runner to
// what happened to be on one machine: a project scaffolded on Go 1.26 fails to
// build on the 1.24 irgo itself targets, with "go.mod requires go >= 1.26.5"
// in a repository that never asked for it. That is what happened to the docs
// site, and it failed in CI rather than for the person who created it.
//
// The full version, not just the major and minor: a module whose go directive
// is lower than a dependency's fails outright.
//
// 1.25 because gomobile requires golang.org/x/mobile in the project's
// dependency graph and x/mobile requires 1.25. irgo builds for iOS and
// Android, so that is irgo's floor too — declaring less would mean every
// mobile build silently rewrote the project's go.mod to raise it.
//
// A test keeps this equal to the framework's own requirement.
const minGoVersion = "1.25.0"

// scaffoldGoVersion is what a generated project should require.
func scaffoldGoVersion() string { return minGoVersion }

// runningGoVersion is the toolchain actually in use, which is a different
// question from what a generated project should require.
//
// go.work has to be at least as high as every module it includes, so writing
// the framework's floor there fails against a project whose go.mod is more
// specific: a workspace saying "go 1.24" over a module saying "go 1.24.12"
// gets "updates to go.mod needed", and the mobile build dies at gomobile with
// no mention of a workspace. Conflating the two broke every iOS and Android
// build in CI while every desktop and web one passed.
func runningGoVersion() string {
	out, err := exec.Command(goBin(), "version").Output()
	if err != nil {
		return minGoVersion
	}
	// "go version go1.24.12 darwin/arm64"
	re := regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`)
	if m := re.FindStringSubmatch(string(out)); len(m) > 1 {
		return m[1]
	}
	return minGoVersion
}

// replaceDirective returns the go.mod `replace` line for the generated project,
// or "" to build against the published upstream module.
//
// IRGO_REPLACE pins a *published* module version — "github.com/joeblew999/irgo
// v0.4.0-androidapi21.24" (an "@" between module and version also works). This
// is what lets a downstream repo regenerate its app entirely from the CLI: the
// fork pin lives in that repo's own config, so the generated go.mod is pure
// output that never needs a hand-edit afterwards.
//
// Otherwise fall back to a local source checkout — the path replace you want
// when hacking on irgo itself.
func replaceDirective() string {
	if mod := strings.TrimSpace(os.Getenv("IRGO_REPLACE")); mod != "" {
		// Accept "module@version" as well as go.mod's own "module version".
		return fmt.Sprintf("\nreplace github.com/stukennedy/irgo => %s\n",
			strings.Replace(mod, "@", " ", 1))
	}
	if path := getIrgoPath(); path != "" {
		return fmt.Sprintf("\nreplace github.com/stukennedy/irgo => %s\n", path)
	}
	return ""
}

// getIrgoPath returns the path to the irgo source directory if developing locally
func getIrgoPath() string {
	// Check IRGO_PATH environment variable first
	if path := os.Getenv("IRGO_PATH"); path != "" {
		return path
	}

	// Helper to check if a directory contains irgo source
	isIrgoDir := func(dir string) bool {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			return strings.Contains(string(data), "module github.com/stukennedy/irgo")
		}
		return false
	}

	// Check if we're running from within or near the irgo source tree
	// by looking at current directory and parents
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 10; i++ {
			if isIrgoDir(dir) {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Check relative to executable location
	// If irgo binary is at /path/to/irgo/cmd/irgo/irgo, source is at /path/to/irgo
	if execPath, err := os.Executable(); err == nil {
		execPath, _ = filepath.EvalSymlinks(execPath)
		execDir := filepath.Dir(execPath)

		// Check if we're in cmd/irgo directory
		if filepath.Base(filepath.Dir(execDir)) == "cmd" {
			possibleRoot := filepath.Dir(filepath.Dir(execDir))
			if isIrgoDir(possibleRoot) {
				return possibleRoot
			}
		}

		// Check parent directories of executable
		dir := execDir
		for i := 0; i < 5; i++ {
			if isIrgoDir(dir) {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Check common development locations
	home, _ := os.UserHomeDir()
	commonPaths := []string{
		filepath.Join(home, "Dev", "@irgo", "core"),
		filepath.Join(home, "Dev", "irgo"),
		filepath.Join(home, "dev", "irgo"),
		filepath.Join(home, "Development", "irgo"),
		filepath.Join(home, "Projects", "irgo"),
		filepath.Join(home, "go", "src", "github.com", "stukennedy", "irgo"),
	}

	for _, path := range commonPaths {
		if isIrgoDir(path) {
			return path
		}
	}

	return ""
}

// sanitizeIdentifier converts a project name into a form usable as a Java/Kotlin
// package fragment or Go package name ({{PROJECT_IDENT}}). Specifically:
//   - non-alphanumeric characters (other than _) are replaced with underscores
//   - leading digits are prefixed with `app` (Java doesn't allow identifiers to
//     start with a digit)
//   - the empty string becomes "app"
//
// Examples: "my-app" -> "my_app", "123game" -> "app123game", "hi" -> "hi".
// For Apple bundle IDs, which reject underscores too, see sanitizeBundleIdent.
func sanitizeIdentifier(name string) string {
	if name == "" {
		return "app"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "app" + result
	}
	if result == "" {
		return "app"
	}
	return result
}

// isRemoteModulePath checks if a path looks like a remote Go module path
func isRemoteModulePath(path string) bool {
	remotePrefixes := []string{
		"github.com/",
		"gitlab.com/",
		"bitbucket.org/",
		"gopkg.in/",
	}
	for _, prefix := range remotePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	// Any path with dots before slashes is likely remote
	if strings.Contains(path, ".") && strings.Contains(path, "/") {
		dotIdx := strings.Index(path, ".")
		slashIdx := strings.Index(path, "/")
		if dotIdx < slashIdx {
			return true
		}
	}
	return false
}

func newProject(name string) error {
	// Determine project directory, project name, and module path
	var projectDir string
	var projectName string
	var modulePath string

	if name == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		projectDir = cwd
		projectName = filepath.Base(cwd)
		modulePath = projectName
	} else if filepath.IsAbs(name) {
		// Absolute path provided
		projectDir = name
		projectName = filepath.Base(name)
		modulePath = projectName
	} else if isRemoteModulePath(name) {
		// Remote module path like "github.com/user/project"
		// Use the last part for directory, full path for module
		projectDir = filepath.Base(name)
		projectName = filepath.Base(name)
		modulePath = name
	} else {
		projectDir = name
		projectName = name
		modulePath = name
	}

	// Check if directory exists and is not empty
	if name != "." {
		if _, err := os.Stat(projectDir); err == nil {
			entries, _ := os.ReadDir(projectDir)
			if len(entries) > 0 {
				return fmt.Errorf("directory %s already exists and is not empty", projectDir)
			}
		}
	}

	fmt.Printf("Creating new irgo project: %s\n", projectName)

	// Create project structure
	dirs := []string{
		"handlers",
		"templates",
		"static/css",
		"static/js",
	}

	for _, dir := range dirs {
		path := filepath.Join(projectDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", path, err)
		}
	}

	// Copy template files
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// go.mod carries the dependency graph and, when a fork is tracked, the
		// replace that selects the CLI — `go tool irgo` reads its version from
		// exactly here. Rewriting it from a template discards both, which then
		// silently changes which CLI the project uses. Seed it only when there
		// is none.
		if strings.HasSuffix(path, "/go.mod.tmpl") || path == "templates/go.mod.tmpl" {
			if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err == nil {
				fmt.Println("  kept (yours): go.mod")
				return nil
			}
		}

		// README.md describes the project, not the framework — a generated one
		// is only a starting point. Regenerating over it would throw away
		// whatever the project actually says about itself.
		if strings.HasSuffix(path, "/README.md.tmpl") || path == "templates/README.md.tmpl" {
			if _, err := os.Stat(filepath.Join(projectDir, "README.md")); err == nil {
				fmt.Println("  kept (yours): README.md")
				return nil
			}
		}

		// irgo.package.toml holds settings a person chose — the signing team,
		// store IDs, the app version. The template version is only a seed, so
		// regenerating in place (irgo project new .) must not overwrite an existing
		// one: that silently discards configuration and the next build fails
		// somewhere unrelated.
		if strings.HasSuffix(path, "/"+packageConfigFile) || path == "templates/"+packageConfigFile {
			if _, err := os.Stat(filepath.Join(projectDir, packageConfigFile)); err == nil {
				fmt.Printf("  kept (yours): %s\n", packageConfigFile)
				return nil
			}
		}

		// appicon.png is the one source image every platform's icons are
		// generated from, and the shipped one is a placeholder. Overwriting it
		// replaces the project's actual icon with that placeholder, and the
		// loss is silent until a build produces an app branded as irgo.
		if path == "templates/appicon.png" {
			if _, err := os.Stat(filepath.Join(projectDir, "appicon.png")); err == nil {
				fmt.Println("  kept (yours): appicon.png")
				return nil
			}
		}

		// The CI workflows live under templates/github but are not part of a
		// new project — `irgo project ci` scaffolds them to .github/ on request.
		// Without this they land in every project as a stray github/ folder
		// that GitHub ignores, so nothing runs and nothing says why.
		if path == "templates/github" {
			return fs.SkipDir
		}

		// Skip the root templates directory
		if path == "templates" {
			return nil
		}

		// Get relative path from templates/
		relPath := strings.TrimPrefix(path, "templates/")
		destPath := filepath.Join(projectDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Read template file
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		// Handle .tmpl extension (remove it) - do this before checking file type
		if strings.HasSuffix(destPath, ".tmpl") {
			destPath = strings.TrimSuffix(destPath, ".tmpl")
			relPath = strings.TrimSuffix(relPath, ".tmpl")
		}

		// Pin irgo to a fork tag (IRGO_REPLACE) or a local checkout — only
		// go.mod carries a replace directive.
		replace := ""
		if strings.HasSuffix(relPath, "go.mod") {
			replace = replaceDirective()
		}
		contentStr := renderTemplate(string(content), projectName, modulePath, replace)

		// Shell scripts and gradle wrappers must be executable
		fileMode := os.FileMode(0644)
		base := filepath.Base(destPath)
		if strings.HasSuffix(base, ".sh") || base == "gradlew" {
			fileMode = 0755
		}

		// Write file
		if err := os.WriteFile(destPath, []byte(contentStr), fileMode); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		fmt.Printf("  created: %s\n", relPath)
		return nil
	})

	if err != nil {
		return fmt.Errorf("copying templates: %w", err)
	}

	// Make scripts executable
	scripts := []string{}
	for _, script := range scripts {
		path := filepath.Join(projectDir, script)
		if err := os.Chmod(path, 0755); err != nil {
			// Ignore if file doesn't exist
			continue
		}
	}

	// Every project wants CI, so scaffold it rather than making it a step
	// people have to know about. `irgo project ci --force` regenerates it later.
	if err := runCIIn(projectDir, modulePath); err != nil {
		fmt.Printf("Warning: could not scaffold CI workflows: %v\n", err)
	}

	// Generate templ files BEFORE tidy. Only the generated _templ.go files
	// import templ directly; tidy run first sees no importer and records templ
	// as `// indirect`, which the first `irgo project assets` then flips back to direct
	// — dirtying go.mod, a generated file, on the very first build.
	// Install templ rather than skipping when it is absent. Skipping is not
	// harmless: tidy then runs with no _templ.go on disk, concludes nothing
	// imports templ, and records it as an indirect dependency — so the
	// generated go.mod depends on whether a tool happened to be installed.
	// That is exactly what made the demo's regen check fail in CI and pass
	// locally.
	if err := ensureGoTool("templ"); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
	if _, err := exec.LookPath("templ"); err == nil {
		fmt.Println("Generating templ files...")
		cmd := exec.Command("templ", "generate")
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: templ generate failed: %v\n", err)
		}
	}

	// Run go mod tidy to download dependencies
	// Skip if it's a remote module path that doesn't exist yet
	if isRemoteModulePath(modulePath) {
		fmt.Println("Skipping go mod tidy (remote module path - run manually after pushing to remote)")
	} else {
		fmt.Println("Running go mod tidy...")
		tidyCmd := exec.Command(goBin(), "mod", "tidy")
		tidyCmd.Dir = projectDir
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			fmt.Printf("Warning: go mod tidy failed: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("Project created successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", projectDir)
	fmt.Println()
	// `go tool`, not a bare binary: go.mod pins the CLI, so this is the only
	// form that runs the version the project actually declares. Nothing to
	// install — not even Node; Tailwind is a binary irgo fetches on demand.
	fmt.Println("  go tool irgo tools doctor  # what can this machine build?")
	fmt.Println("  go tool irgo server dev    # dev server with hot reload")
	fmt.Println("  go tool irgo app run ios   # iOS Simulator")
	fmt.Println("  go tool irgo app run android   # Android Emulator")
	fmt.Println("  go tool irgo app run desktop   # native desktop window")
	fmt.Println()
	fmt.Println("Everything else is in README.md.")
	fmt.Println()

	return nil
}

// scaffoldExamples writes the canonical mobile example apps (ios/Example and
// android/Example) into the CURRENT directory from the embedded templates.
// It is missing-only and idempotent — an existing Example project is never
// overwritten, so build/run can call it unconditionally. This is what makes
// `irgo app build/run android|ios` work on a bare project: devs and CI never
// hand-copy the native shells, and never need a separate scaffold step.
func scaffoldExamples() error {
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}
	projectName := filepath.Base(modulePath)
	if projectName == "." || projectName == "" || projectName == "/" {
		wd, _ := os.Getwd()
		projectName = filepath.Base(wd)
	}
	if projectName == "." || projectName == "" {
		projectName = "irgo"
	}

	fmt.Println("Ensuring mobile example projects (ios/Example, android/Example)...")
	for _, sub := range []string{"ios/Example", "android/Example"} {
		if err := scaffoldExampleDir(sub, projectName, modulePath); err != nil {
			return err
		}
	}
	return nil
}

// scaffoldExampleDir copies one example subtree (rel, e.g. "ios/Example") from
// the embedded templates into the current directory, resolving the same
// placeholders as irgo project new. Skips (without error) when the destination already
// exists so it is safe to call on every build/run.
func scaffoldExampleDir(rel, projectName, modulePath string) error {
	if _, err := os.Stat(rel); err == nil {
		fmt.Printf("  exists (skipped): %s\n", rel)
		return nil
	}

	written := 0
	srcPrefix := "templates/" + rel
	err := fs.WalkDir(templateFS, srcPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		destPath := strings.TrimPrefix(path, "templates/")
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		// Handle .tmpl extension (remove it), like irgo project new.
		if strings.HasSuffix(destPath, ".tmpl") {
			destPath = strings.TrimSuffix(destPath, ".tmpl")
		}

		contentStr := renderTemplate(string(content), projectName, modulePath, "")

		// Shell scripts and gradle wrappers must be executable.
		fileMode := os.FileMode(0o644)
		base := filepath.Base(destPath)
		if strings.HasSuffix(base, ".sh") || base == "gradlew" {
			fileMode = 0o755
		}

		if err := os.WriteFile(destPath, []byte(contentStr), fileMode); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}
		written++
		fmt.Printf("  created: %s\n", destPath)
		return nil
	})
	if err != nil {
		return err
	}
	if written == 0 {
		return fmt.Errorf("no files scaffolded for %s (template missing?)", rel)
	}
	fmt.Printf("  scaffolded %d files into %s\n", written, rel)
	return nil
}
