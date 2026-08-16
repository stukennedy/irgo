// Core commands: the dev server, plain serve, and the test runner.
package main

import (
	"fmt"
	"os"
)

// runDev starts the development server with hot reload.
//
// air rebuilds on change using .air.toml, which calls `irgo project assets` — so the
// generate step lives in one place rather than being restated per project.
func runDev() error {
	if err := ensureGoTool("air"); err != nil {
		return err
	}
	if err := ensureGoTool("templ"); err != nil {
		return err
	}
	// Generate once up front so the first build is correct; air regenerates on
	// every change after that.
	if err := ensureAssets(); err != nil {
		return err
	}

	if _, err := os.Stat("main.go"); err != nil {
		// Framework checkout rather than a generated project.
		if _, e := os.Stat("examples/todo/main.go"); e != nil {
			return fmt.Errorf("no main.go found - are you in an irgo project?")
		}
	}

	fmt.Println("Starting development server (http://localhost:8080)...")
	return runCommand("air")
}

// runServe starts the server without file watching
func runServe() error {
	// Check if main.go exists
	if _, err := os.Stat("main.go"); err == nil {
		// User project
		return runCommand(goBin(), "run", ".", "serve")
	}

	// Framework - run example
	if _, err := os.Stat("examples/todo/main.go"); err == nil {
		return runCommand(goBin(), "run", "./examples/todo", "serve")
	}

	return fmt.Errorf("no main.go found - are you in an irgo project?")
}

// runTest runs the test suite
func runTest() error {
	// Generate first. _templ.go and output.css are gitignored, so on a fresh
	// clone the templates package has no Go files at all and `go test` fails
	// with "package <mod>/templates is not in std" — which names neither templ
	// nor the missing step. The help has always said this command regenerates;
	// it did not, and CI trusted the documentation.
	if err := ensureAssets(); err != nil {
		return err
	}
	if err := runPreTestSteps(); err != nil {
		return err
	}

	fmt.Println("Running tests...")
	return runCommand(goBin(), "test", "./...")
}

func init() {
	register(command{
		noun: "project", verb: "test", order: 60,
		summary: "Run the tests",
		usage:   [][2]string{{"", "Regenerate assets, then go test ./..."}},
		notes: `Assets first, so tests that render templates see the current ones rather than
whatever was last on disk.`,
	})
}

func init() {
	register(command{
		noun: "server", verb: "dev", order: 0,
		summary: "Web server with hot reload",
		usage:   [][2]string{{"", "Serve on :8080 and rebuild on save"}},
		notes: `Rebuilds templ, Tailwind and the Go binary as you save. From an Android
emulator the same server is http://10.0.2.2:8080, which is what
irgo app run android --dev connects to.

air is installed on demand.`,
	})
}

func init() {
	register(command{
		noun: "server", verb: "serve", order: 10,
		summary: "Web server without file watching",
		usage:   [][2]string{{"", "Serve on :8080"}},
		notes: `Regenerates assets once at startup and then leaves them alone — for checking a
build, or running the web target.`,
	})
}
