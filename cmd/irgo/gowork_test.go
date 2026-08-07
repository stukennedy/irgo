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
		// The custom module must be live: on a workspace carrying irgo's
		// membership signature (the temp-clone entry), a vanished entry is
		// only a dangling reference and no longer protects (round 19).
		live := filepath.Join(dir, "some-local-module")
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(live, "go.mod"),
			[]byte("module example.com/local\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write("extra.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", mobileDir, live))
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

	t.Run("marked unparseable file is protected", func(t *testing.T) {
		// go work edit preserves the marker, so a broken file with it may
		// still be a customized workspace mid-edit; unparseable content
		// cannot be shown to be irgo-only, so it is protected regardless
		// of the marker (round 20).
		path := write("brokenmarked.work", goWorkGeneratedMarker+"\nuse (\n\tnever closed\n")
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected marked unparseable go.work to be protected")
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

	t.Run("marked file with live custom use entry is protected", func(t *testing.T) {
		// go work edit preserves comments, so a developer can customize a
		// generated workspace without ever removing the marker (round 14).
		live := filepath.Join(dir, "live-custom")
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(live, "go.mod"),
			[]byte("module example.com/live\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write("marked.work", goWorkGeneratedMarker+
			fmt.Sprintf("\n\ngo 1.24\n\nuse (\n\t.\n\t%s\n)\n", live))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected customized marked go.work to be protected")
		}
	})

	t.Run("marked file with vanished member is regenerable", func(t *testing.T) {
		// A marked workspace whose generated irgo checkout was deleted must
		// self-heal (round 18): the vanished entry is a dangling reference,
		// so regeneration loses nothing recoverable.
		gone := filepath.Join(dir, "deleted-checkout")
		path := write("markedgone.work", goWorkGeneratedMarker+
			fmt.Sprintf("\n\ngo 1.24\n\nuse (\n\t.\n\t%s\n)\n", gone))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected marked go.work with vanished member to be regenerable")
		}
	})

	t.Run("marked file with replace directive is protected", func(t *testing.T) {
		path := write("markedreplace.work", goWorkGeneratedMarker+
			"\n\ngo 1.24\n\nuse .\n\nreplace example.com/a => ../a\n")
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected marked go.work with replace to be protected")
		}
	})

	t.Run("marked irgo-shaped file is regenerable", func(t *testing.T) {
		mobileDir := filepath.Join(os.TempDir(), "golang-mobile")
		path := write("markedclean.work", goWorkGeneratedMarker+
			fmt.Sprintf("\n\ngo 1.24\n\nuse (\n\t.\n\t%s\n)\n", mobileDir))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected unmodified marked go.work to be regenerable")
		}
	})

	t.Run("vanished custom module named golang-mobile without marker is customized when other custom entries exist", func(t *testing.T) {
		// Codex round-5 scenario: a customized workspace with a removed
		// module that happens to be named golang-mobile. Without the
		// marker, a live extra custom entry keeps the file protected —
		// live, because a vanished one would be only a dangling
		// reference on a workspace bearing irgo's clone signature
		// (round 19).
		gone := filepath.Join(dir, "vendor-forks", "golang-mobile")
		live := filepath.Join(dir, "their-other-module")
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(live, "go.mod"),
			[]byte("module example.com/theirs\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write("nomarker.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", gone, live))
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

	// A corrupted checkout (valid module line, malformed body) must not
	// count as provenance either (round 13: strict parse everywhere).
	corruptCheckout := filepath.Join(dir, "corrupt-irgo")
	if err := os.MkdirAll(corruptCheckout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptCheckout, "go.mod"),
		[]byte("module github.com/stukennedy/irgo\n\nrequire (\n\texample.com/dep"), 0o644); err != nil {
		t.Fatal(err)
	}
	content = fmt.Sprintf("go 1.24\n\nuse (\n\t.\n\t%s\n)\n", corruptCheckout)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if goWorkIrgoGenerated(path) {
		t.Fatal("expected corrupted checkout go.mod to be rejected as provenance")
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

// An unmarked legacy workspace whose generated irgo checkout AND temp clone
// were both deleted must still self-heal: the temp-clone use entry is irgo's
// membership signature, standing in for the marker on pre-marker files
// (review round 19).
func TestGoWorkIrgoGeneratedLegacyVanishedCheckout(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	goneCheckout := filepath.Join(dir, "deleted-irgo-checkout")

	t.Run("vanished checkout with vanished old-TMPDIR clone is regenerable", func(t *testing.T) {
		goneClone := filepath.Join(dir, "old-tmp", "golang-mobile")
		path := write("legacy.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", goneCheckout, goneClone))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected legacy go.work with vanished generated members to be regenerable")
		}
	})

	t.Run("vanished checkout with current temp clone entry is regenerable", func(t *testing.T) {
		mobileDir := filepath.Join(os.TempDir(), "golang-mobile")
		path := write("legacy2.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", goneCheckout, mobileDir))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected legacy go.work with vanished checkout to be regenerable")
		}
	})

	t.Run("vanished entry without any clone signature is protected", func(t *testing.T) {
		// No marker and no golang-mobile member: nothing ties this file to
		// irgo, so a vanished entry may be a customized workspace's deleted
		// module and must not authorize regeneration.
		path := write("custom.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n)\n", goneCheckout))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected unmarked go.work without clone signature to stay protected")
		}
	})

	t.Run("vanished checkout with live old-TMPDIR x/mobile clone is regenerable", func(t *testing.T) {
		// The prior session's clone still exists on disk but under an old
		// TMPDIR: a live x/mobile checkout (strict parse, exact module) is
		// irgo's clone all the same and carries the legacy signature
		// (round 21).
		liveClone := filepath.Join(dir, "live-old-tmp", "golang-mobile")
		if err := os.MkdirAll(liveClone, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(liveClone, "go.mod"),
			[]byte("module golang.org/x/mobile\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write("legacy3.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", goneCheckout, liveClone))
		if !goWorkIrgoGenerated(path) {
			t.Fatal("expected legacy go.work with live old x/mobile clone to be regenerable")
		}
	})

	t.Run("live golang-mobile dir of another module is not a signature", func(t *testing.T) {
		// A real custom module that merely sits in a directory named
		// golang-mobile must not grant the legacy signature or be skipped
		// as irgo's member.
		imposter := filepath.Join(dir, "forks", "golang-mobile")
		if err := os.MkdirAll(imposter, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(imposter, "go.mod"),
			[]byte("module example.com/mymobile\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := write("imposter.work", fmt.Sprintf(
			"go 1.24\n\nuse (\n\t.\n\t%s\n\t%s\n)\n", goneCheckout, imposter))
		if goWorkIrgoGenerated(path) {
			t.Fatal("expected non-x/mobile golang-mobile dir to leave the file protected")
		}
	})
}

// mobileCloneUsable must reject directories that are not the pinned
// golang.org/x/mobile checkout: an unrelated module squatting the temp path
// would otherwise have its go.mod rewritten and be used as a tool source,
// and a right-module/wrong-revision clone would defeat pinXMobile.
func TestMobileCloneUsable(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing dir is unusable", func(t *testing.T) {
		if mobileCloneUsable(filepath.Join(dir, "nope")) {
			t.Fatal("expected missing dir to be unusable")
		}
	})

	t.Run("unrelated module is unusable", func(t *testing.T) {
		other := filepath.Join(dir, "other")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, "go.mod"),
			[]byte("module example.com/other\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if mobileCloneUsable(other) {
			t.Fatal("expected unrelated module to be unusable")
		}
	})

	t.Run("right module without pinned git revision is unusable", func(t *testing.T) {
		fake := filepath.Join(dir, "fake-xmobile")
		if err := os.MkdirAll(fake, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fake, "go.mod"),
			[]byte("module golang.org/x/mobile\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// No git repo at all — rev-parse fails, so the pin cannot be verified.
		if mobileCloneUsable(fake) {
			t.Fatal("expected unverifiable revision to be unusable")
		}
	})
}
