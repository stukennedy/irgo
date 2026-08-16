package datastarjs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestVendoredClientIsAStableRelease — every generated project ran
// v1.0.0-RC.7 against a stable v1.1.0 of the Go library, because the client
// was downloaded from a CDN at scaffold time and nothing ever looked at it
// again. A release candidate is not something to ship by default.
func TestVendoredClientIsAStableRelease(t *testing.T) {
	if !strings.HasPrefix(Version, "v") {
		t.Fatalf("Version %q should look like v1.2.3", Version)
	}
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Errorf("Version %q is not a stable release — projects should not ship an RC by default", Version)
	}
}

// TestClientIsActuallyEmbedded guards the failure that would be invisible: an
// empty or truncated bundle still compiles and serves, and the page simply
// does nothing.
func TestClientIsActuallyEmbedded(t *testing.T) {
	if len(script) < 10_000 {
		t.Fatalf("the embedded client is %d bytes — too small to be Datastar", len(script))
	}
	body := string(script)
	for _, want := range []string{"datastar-patch-elements", "datastar-patch-signals"} {
		if !strings.Contains(body, want) {
			t.Errorf("the embedded client never mentions %q — it cannot apply what the Go side sends", want)
		}
	}
}

// TestGoAndClientAgreeOnTheWireFormat is the pairing this package exists for.
//
// The two halves are separate dependencies and datastar-go publishes no
// version constant, so nothing but this checks that the events the library
// writes are the events the client listens for.
func TestGoAndClientAgreeOnTheWireFormat(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Skip("not in the framework repository")
	}
	if !strings.Contains(string(mod), "starfederation/datastar-go") {
		t.Skip("datastar-go is not a dependency here")
	}

	// The literals datastar-go writes into the SSE stream.
	for _, event := range []string{"datastar-patch-elements", "datastar-patch-signals"} {
		if !strings.Contains(string(script), event) {
			t.Errorf("datastar-go emits %s and the embedded client does not handle it — "+
				"the versions have diverged", event)
		}
	}
}

// TestServedClientHasNoDanglingSourceMap — the bundle ships with a
// //# sourceMappingURL comment and the map is not distributed with it. A
// browser only resolves that when devtools are open, so every generated
// project served a dead reference without anyone noticing. Anything that
// resolves it eagerly fails to compile the client at all.
func TestServedClientHasNoDanglingSourceMap(t *testing.T) {
	if strings.Contains(string(Script()), "sourceMappingURL") {
		t.Error("the served client still references a source map that is not shipped")
	}
	if !strings.Contains(string(Script()), "datastar") {
		t.Error("stripping the source map appears to have eaten the bundle")
	}
}
