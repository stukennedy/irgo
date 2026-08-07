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

	t.Run("all referenced dirs exist", func(t *testing.T) {
		a := filepath.Join(dir, "mod-a")
		b := filepath.Join(dir, "mod-b")
		if err := os.MkdirAll(a, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(b, 0o755); err != nil {
			t.Fatal(err)
		}
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", a, b))
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (all dirs exist)")
		}
	})

	t.Run("missing referenced dir", func(t *testing.T) {
		a := filepath.Join(dir, "mod-a")
		if err := os.MkdirAll(a, 0o755); err != nil {
			t.Fatal(err)
		}
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
		a := filepath.Join(dir, "mod-comment")
		if err := os.MkdirAll(a, 0o755); err != nil {
			t.Fatal(err)
		}
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t. // root module\n\t%s // temp clone\n)\n", a))
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (inline comments must not read as paths)")
		}
	})

	t.Run("single-line use directive", func(t *testing.T) {
		a := filepath.Join(dir, "mod-single")
		if err := os.MkdirAll(a, 0o755); err != nil {
			t.Fatal(err)
		}
		path := write(fmt.Sprintf("go 1.24\n\nuse %s\n", a))
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (single-line use)")
		}
	})

	t.Run("quoted relative path resolves against go.work dir", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(dir, "rel mod"), 0o755); err != nil {
			t.Fatal(err)
		}
		path := write("go 1.24\n\nuse \"./rel mod\"\n")
		if !goWorkFileValid(path) {
			t.Fatal("expected valid go.work (quoted relative path)")
		}
	})

	t.Run("unparseable file is invalid", func(t *testing.T) {
		path := write("use (\n\tnever closed\n")
		if goWorkFileValid(path) {
			t.Fatal("expected invalid go.work (unparseable)")
		}
	})
}
