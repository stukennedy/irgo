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
		// A fixed valid module path: directory names like "rel mod" are not
		// legal module paths, and isModuleDir now does a strict parse.
		if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module example.com/testmod\n"), 0o644); err != nil {
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

	t.Run("truncated go.mod in use target is invalid", func(t *testing.T) {
		// An interrupted checkout can leave a regular but malformed go.mod
		// (no module directive); Go rejects it, so the workspace is stale.
		trunc := filepath.Join(dir, "truncated-mod")
		if err := os.MkdirAll(trunc, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(trunc, "go.mod"), []byte("modu"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", trunc))
		if goWorkFileValid(path) {
			t.Fatal("expected invalid go.work (use target go.mod has no module directive)")
		}
	})

	t.Run("go.mod with valid module line but malformed body is invalid", func(t *testing.T) {
		// modfile.ModulePath would accept this; Go's loader does not.
		corrupt := filepath.Join(dir, "corrupt-mod")
		if err := os.MkdirAll(corrupt, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(corrupt, "go.mod"),
			[]byte("module example.com/corrupt\n\nrequire (\n\texample.com/dep"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write(fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", corrupt))
		if goWorkFileValid(path) {
			t.Fatal("expected invalid go.work (use target go.mod fails a full parse)")
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

	t.Run("unmarked unparseable file is protected", func(t *testing.T) {
		// A parse failure on an unmarked file carries no provenance: it may
		// be a developer's unfinished edit of a customized workspace, so it
		// must not be deleted.
		path := write("broken.work", "use (\n\tnever closed\n")
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected unmarked unparseable go.work to be protected")
		}
	})

	t.Run("marked unparseable file is regenerable", func(t *testing.T) {
		path := write("brokenmarked.work", goWorkGeneratedMarker+"\nuse (\n\tnever closed\n")
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected marked go.work to be regenerable even when broken")
		}
	})
}

// Regression tests for the round-4 review findings: TMPDIR drift and
// toolchain/godebug directives.
func TestGoWorkIrgoGeneratedEdgeCases(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	t.Run("golang-mobile from an earlier TMPDIR is regenerable", func(t *testing.T) {
		// The clone path of a previous session whose temp dir was cleaned:
		// not the current os.TempDir(), and it no longer exists.
		oldClone := filepath.Join(dir, "old-tmp", "golang-mobile")
		path := write("oldtmp.work", fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", oldClone))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected go.work referencing a vanished golang-mobile clone to be regenerable")
		}
	})

	t.Run("existing custom golang-mobile dir marks it customized", func(t *testing.T) {
		custom := filepath.Join(dir, "golang-mobile")
		if err := os.MkdirAll(custom, 0o755); err != nil {
			t.Fatal(err)
		}
		path := write("livemobile.work", fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", custom))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected a live custom golang-mobile dir to mark the file customized")
		}
	})

	t.Run("toolchain directive marks it customized", func(t *testing.T) {
		path := write("toolchain.work", "go 1.24\n\ntoolchain go1.24.5\n\nuse .\n")
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected toolchain directive to mark the file customized")
		}
	})

	t.Run("godebug directive marks it customized", func(t *testing.T) {
		path := write("godebug.work", "go 1.24\n\ngodebug default=go1.23\n\nuse .\n")
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected godebug directive to mark the file customized")
		}
	})
}

// The generation marker is definitive provenance: files carrying it are
// regenerable no matter what they contain, and new files always carry it.
func TestGoWorkGeneratedMarker(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	t.Run("marker makes any content regenerable", func(t *testing.T) {
		path := write("marked.work", goWorkGeneratedMarker+
			"\n\ngo 1.24\n\nuse (\n\t.\n\t../some-custom-module\n)\n")
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected marker to be definitive provenance")
		}
	})

	t.Run("vanished custom module named golang-mobile without marker is customized when other custom entries exist", func(t *testing.T) {
		// Codex round-5 scenario: a customized workspace with a removed
		// module that happens to be named golang-mobile. Without the
		// marker, the extra custom entry keeps the file protected.
		gone := filepath.Join(dir, "vendor-forks", "golang-mobile")
		path := write("nomarker.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t../their-other-module\n)\n", gone))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected unmarked customized file to stay protected")
		}
	})
}

// A use path replaced by a regular file makes Stat(go.mod) fail with
// ENOTDIR — not ENOENT — and the module is just as unloadable. The
// workspace must count as stale (review round 7).
func TestGoWorkFileValidUseTargetIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module root\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	notADir := filepath.Join(dir, "was-a-module")
	if err := os.WriteFile(notADir, []byte("just a file"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "go.work")
	content := fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", notADir)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if goWorkFileValid(path) {
		t.Fatal("expected invalid go.work (use target is a file, ENOTDIR)")
	}
}

// A legacy go.work may reference an irgo checkout discovered by a previous
// CLI that the current getIrgoPath() no longer finds; its go.mod module
// declaration identifies it as irgo's own member (review round 8).
func TestGoWorkIrgoGeneratedLegacyIrgoCheckout(t *testing.T) {
	dir := t.TempDir()

	checkout := filepath.Join(dir, "some-old-irgo-checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "go.mod"),
		[]byte("module github.com/stukennedy/irgo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "go.work")
	content := fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", checkout)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if !goWorkIrgoGenerated(path) {
		t.Fatal("expected legacy irgo checkout entry to be recognized as irgo's own")
	}

	// A module whose path merely contains the irgo path as a prefix must
	// not count as provenance (round 9: exact match, not substring).
	tools := filepath.Join(dir, "irgo-tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "go.mod"),
		[]byte("module github.com/stukennedy/irgo-tools\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content = fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", tools)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if goWorkIrgoGenerated(path) {
		t.Fatal("expected prefix-matching module path to mark the file customized")
	}

	// A live checkout of some other module stays customized.
	other := filepath.Join(dir, "other-module")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "go.mod"),
		[]byte("module example.com/other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content = fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", other)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if goWorkIrgoGenerated(path) {
		t.Fatal("expected non-irgo module entry to mark the file customized")
	}
}
