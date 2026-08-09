package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Components can live in a dependency, and Tailwind only scans the project.
// Without the generated @source lines they render with classes no stylesheet
// defines: the markup is correct and the page is unstyled, which reads as a
// broken component rather than a missing scan path.

// TestNoDependencyComponentsUsesInputDirectly — a project with nothing to add
// must build from its own input.css, not a copy of it. A temporary file that
// exists for every project is a temporary file that will eventually be left
// behind.
func TestNoDependencyComponentsUsesInputDirectly(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.css")
	if err := os.WriteFile(in, []byte("@import \"tailwindcss\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inProjectDir(t, dir)

	entry, cleanup, err := cssEntryWithDependencySources(in)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if entry != in {
		t.Errorf("entry = %q, want the project's own %q", entry, in)
	}
}

// TestGeneratedEntryKeepsTheProjectsOwnCSS — the project's stylesheet has to
// survive intact, and the @source lines have to precede it.
func TestGeneratedEntryKeepsTheProjectsOwnCSS(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.css")
	const own = "@import \"tailwindcss\";\n.mine { color: red }\n"
	if err := os.WriteFile(in, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	// Exercise the writer directly: resolving real dependencies needs a real
	// module, and what matters here is that nothing of the project is lost.
	entry, cleanup, err := writeCSSEntry(in, []byte(own), []string{"/pkg/mod/example/button"})
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)

	if !strings.Contains(got, own) {
		t.Error("the project's own CSS did not survive into the generated entry")
	}
	if !strings.Contains(got, `@source "/pkg/mod/example/button";`) {
		t.Errorf("no @source for the dependency:\n%s", got)
	}
	if strings.Index(got, "@source") > strings.Index(got, ".mine") {
		t.Error("@source must come before the project's rules")
	}
	// Beside input.css, or the relative @import paths inside it stop resolving.
	if filepath.Dir(entry) != filepath.Dir(in) {
		t.Errorf("entry at %s, not beside %s", entry, in)
	}
}

// TestGeneratedEntryIsRemoved — it carries machine-specific module cache paths
// and must never be left in a project, let alone committed.
func TestGeneratedEntryIsRemoved(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.css")
	if err := os.WriteFile(in, []byte("@import \"tailwindcss\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, cleanup, err := writeCSSEntry(in, []byte("x"), []string{"/pkg/mod/example/button"})
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Errorf("%s survived cleanup", entry)
	}
}

// TestProjectCSSIsNeverEdited — input.css belongs to the project. Writing a
// module cache path into it would break on the next machine and on every
// version bump.
func TestProjectCSSIsNeverEdited(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.css")
	const own = "@import \"tailwindcss\";\n"
	if err := os.WriteFile(in, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := writeCSSEntry(in, []byte(own), []string{"/pkg/mod/example/button"})
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	after, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != own {
		t.Errorf("input.css was modified:\n%s", after)
	}
}

func inProjectDir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
}
