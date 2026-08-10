// Scaffolding CI into a project.
//
// Every target irgo supports needs a runner of the right OS — iOS needs macOS,
// the Linux desktop needs Linux, Windows desktop builds natively. Working that
// out, and keeping it correct as the CLI changes, is not a job each project
// should redo. The workflows ship as templates for the same reason the native
// shells do: they are output of the CLI, not something to hand-maintain.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/github
var ciTemplates embed.FS

// runCI writes .github/workflows into the current project. Existing files are
// left alone unless force is set, so re-running after an upgrade is safe and
// local edits are never silently discarded.
func runCI(force bool) error {
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("no go.mod here — run irgo project ci from your project root")
	}
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}
	return writeCIWorkflows(".", modulePath, force, true)
}

// runCIIn scaffolds CI into a freshly created project. Quiet and never
// overwriting, since nothing can exist yet.
func runCIIn(projectDir, modulePath string) error {
	return writeCIWorkflows(projectDir, modulePath, false, false)
}

func writeCIWorkflows(projectDir, modulePath string, force, verbose bool) error {

	var written, skipped int
	err := fs.WalkDir(ciTemplates, "templates/github", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, "templates/github/")
		dest := filepath.Join(projectDir, ".github", rel)

		if _, err := os.Stat(dest); err == nil && !force {
			if verbose {
				fmt.Printf("  exists (skipped): %s\n", dest)
			}
			skipped++
			return nil
		}
		data, err := ciTemplates.ReadFile(path)
		if err != nil {
			return err
		}
		body := renderCITemplate(string(data), filepath.Base(modulePath))

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
			return err
		}
		if verbose {
			fmt.Printf("  created: %s\n", dest)
		}
		written++
		return nil
	})
	if err != nil {
		return err
	}

	if !verbose {
		fmt.Printf("  created: .github/workflows (build + release)\n")
		return nil
	}
	fmt.Printf("\n%d workflow(s) written, %d skipped.\n", written, skipped)
	if skipped > 0 {
		fmt.Println("Re-run with --force to overwrite.")
	}
	fmt.Println()
	fmt.Println("The build workflow needs no secrets. For release packaging, see")
	fmt.Println("what each store wants:  irgo app package setup --check")
	return nil
}

func init() {
	register(command{
		noun: "project", verb: "ci", order: 40,
		summary: "Scaffold the GitHub Actions workflows",
		args:    "[--force]",
		usage: [][2]string{
			{"", "Write .github/workflows, keeping any that exist"},
			{"--force", "Regenerate them"},
		},
		notes: `Writes build.yml (every target, on the OS each one needs) and release.yml
(signed store artifacts). Both are generated from the CLI — the desktop matrix,
the artifact paths and the action versions come from irgo, so they stay correct
as it changes. Edit the template, not the output.

Two jobs in build.yml are opt-in, off unless the repository sets a variable:
  IRGO_TOOLCHAIN_ROUNDTRIP=true   install → doctor → build → uninstall, and
                                  assert the machine comes back clean
  IRGO_GENERATED_REPO=true        assert the repo still matches project new

The round-trip deletes the Android toolchain to prove the uninstall works, so
it refuses to run anywhere but a github-hosted runner.`,
	})
}
