package main

import (
	"strings"
	"testing"
)

func TestSanitizeIdentifier(t *testing.T) {
	cases := map[string]string{
		"myapp":           "myapp",
		"my-app":          "my_app",
		"my_app":          "my_app",
		"My App":          "My_App",
		"123game":         "app123game",
		"":                "app",
		"---":             "___",
		"irgo-xtool-test": "irgo_xtool_test",
	}
	for in, want := range cases {
		if got := sanitizeIdentifier(in); got != want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUnpublishedPinIsExplained — the failure a new user meets first.
//
// A scaffolded project pins the CLI's own version, and when that has not been
// tagged, `go mod tidy` fails with "unknown revision". That reads as a broken
// template rather than an unpublished release, and irgo then says "Project
// created successfully!" a moment later.
func TestUnpublishedPinIsExplained(t *testing.T) {
	real := "go: downloading...\ngithub.com/stukennedy/irgo@v0.4.0: reading " +
		"github.com/stukennedy/irgo/go.mod at revision v0.4.0: unknown revision v0.4.0\n"

	if out := captureStdout(t, func() { explainTidyFailure(real) }); out == "" {
		t.Error("the unpublished-pin failure produced no explanation")
	} else if !strings.Contains(out, "project pin") {
		t.Errorf("explanation does not name the remedy:\n%s", out)
	}

	// Anything else must stay quiet: a message that appears for unrelated
	// failures teaches people to skip it.
	for _, other := range []string{
		"go: updates to go.mod needed\n",
		"github.com/other/thing@v1.0.0: unknown revision v1.0.0\n",
		"",
	} {
		if out := captureStdout(t, func() { explainTidyFailure(other) }); out != "" {
			t.Errorf("explained an unrelated failure %q:\n%s", other, out)
		}
	}
}
