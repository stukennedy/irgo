package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// goWorkFileValid must report whether every directory referenced by a go.work
// file still exists — the stale-go.work detection that ensureMobileBuildSetup
// uses to regenerate a go.work pointing at a cleaned-up temp x/mobile clone.
// go.work files written by irgo reference absolute paths (the project root via
// getIrgoPath and os.TempDir()/golang-mobile), so absolute paths are used here
// too, keeping the test independent of the process cwd.
func TestGoWorkFileValid(t *testing.T) {
	dir := t.TempDir()

	write := func(content string) string {
		path := filepath.Join(dir, "go.work")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write go.work: %v", err)
		}
		return path
	}

	// A go.work use target must be a module directory, so test fixtures
	// need a go.mod, not just a directory.
	mkmod := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// The go.work fixtures reference "." — make dir itself a module.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module root\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("all referenced dirs exist", func(t *testing.T) {
		a := mkmod("mod-a")
		b := mkmod("mod-b")
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", a, b))
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (all dirs exist)")
		}
	})

	t.Run("missing referenced dir", func(t *testing.T) {
		a := mkmod("mod-a")
		missing := filepath.Join(dir, "does-not-exist")
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", a, missing))
		if goWorkFileValid(path) {
			t.Fatal("expected invalid go.work (references a missing dir)")
		}
	})

	t.Run("no use block", func(t *testing.T) {
		path := write("go 1.24\n")
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (nothing to check)")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if goWorkFileValid(filepath.Join(dir, "nope.work")) {
			t.Fatal("expected invalid (file does not exist)")
		}
	})

	// Real work files use forms a naive line-splitter misreads; none of
	// these may cause a valid workspace to be treated as stale (and deleted).
	t.Run("inline comment in use block", func(t *testing.T) {
		a := mkmod("mod-comment")
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t. // root module\n\t%s // temp clone\n)\n", a))
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (inline comments must not read as paths)")
		}
	})

	t.Run("single-line use directive", func(t *testing.T) {
		a := mkmod("mod-single")
		path := write(fmt.Sprintf("go 1.24\n\nuse %s\n", a))
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (single-line use)")
		}
	})

	t.Run("quoted relative path resolves against go.work dir", func(t *testing.T) {
		mkmod("rel mod")
		path := write("go 1.24\n\nuse \"./rel mod\"\n")
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (quoted relative path)")
		}
	})

	t.Run("existing dir without go.mod is invalid", func(t *testing.T) {
		// A partial x/mobile clone (process killed between MkdirAll and
		// checkout) exists on disk but is not a module — Go cannot load it,
		// so the workspace must count as stale.
		partial := filepath.Join(dir, "partial-clone")
		if err := os.MkdirAll(partial, 0o755); err != nil {
			t.Fatal(err)
		}
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", partial))
		if goWorkFileValid(path) {
			t.Fatal("expected invalid go.work (use target has no go.mod)")
		}
	})

	t.Run("unparseable file is invalid", func(t *testing.T) {
		path := write("use (\n\tnever closed\n")
		if goWorkFileValid(path) {
			t.Fatal("expected invalid go.work (unparseable)")
		}
	})
}

// goWorkIrgoGenerated gates the wholesale delete-and-regenerate repair: only
// files containing exactly the members irgo writes may be destroyed. A
// customized workspace (replace directives, extra use entries) is
// unrecoverable — go.work is gitignored in generated projects.
func TestGoWorkIrgoGenerated(t *testing.T) {
	dir := t.TempDir()
	mobileDir := filepath.Join(os.TempDir(), "golang-mobile")

	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	t.Run("irgo-shaped file is regenerable", func(t *testing.T) {
		path := write("irgo.work", fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", mobileDir))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected irgo-shaped go.work to be regenerable")
		}
	})

	t.Run("replace directive marks it customized", func(t *testing.T) {
		path := write("replace.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n)\n\nreplace example.com/a => ../a\n", mobileDir))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected replace directive to mark the file customized")
		}
	})

	t.Run("unknown use entry marks it customized", func(t *testing.T) {
		path := write("extra.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t../some-local-module\n)\n", mobileDir))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected extra use entry to mark the file customized")
		}
	})

	t.Run("unparseable file is regenerable", func(t *testing.T) {
		path := write("broken.work", "use (\n\tnever closed\n")
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected unparseable go.work to be regenerable (go rejects it anyway)")
		}
	})
}
