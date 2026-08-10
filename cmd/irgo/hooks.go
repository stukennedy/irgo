// Build and test steps a feature contributes, registered rather than appended.
//
// ensureAssets and runTest are pipelines: regenerate templ, sync a kit's
// stylesheets, sync the agent skills, build the CSS, install a browser, run
// the tests. Every feature that adds a step used to edit the function — which
// made two unrelated features collide in a file neither of them owns, for no
// reason other than both having something to do at the same moment.
//
// It is the same problem as the command table and has the same answer: a step
// is declared by the code that performs it.
package main

import "sort"

// assetStep is work done by `irgo project assets`, which every build runs.
type assetStep struct {
	order int // when, relative to other steps
	run   func() error
}

var assetSteps []assetStep

// Ordering constants, so a step declares when it runs without knowing what
// else exists. Spaced, so something can be put between two of them later
// without renumbering — and named, because "after the kit, before the CSS" is
// the actual requirement and a bare integer does not say it.
const (
	assetOrderKit  = 100 // a UI kit's stylesheets, before Tailwind reads them
	assetOrderDocs = 200 // reference material, which nothing else depends on
	assetOrderLate = 900
)

// registerAssetStep adds a step to every build.
func registerAssetStep(order int, run func() error) {
	assetSteps = append(assetSteps, assetStep{order, run})
}

// runAssetSteps runs them in order, stopping at the first failure.
//
// Sorted by the declared order rather than registration order: registration is
// init order, which is filename order, which is arbitrary — and Tailwind must
// see a kit's stylesheets before it builds, so this one genuinely matters.
func runAssetSteps() error {
	steps := make([]assetStep, len(assetSteps))
	copy(steps, assetSteps)
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].order < steps[j].order })
	for _, s := range steps {
		if err := s.run(); err != nil {
			return err
		}
	}
	return nil
}

// preTestSteps run before `irgo project test` runs anything.
//
// For provisioning that only some projects need — a browser, when a test
// actually asks for one — so the cost is paid by the projects that use it and
// nobody else waits for it.
var preTestSteps []func() error

func registerPreTestStep(run func() error) {
	preTestSteps = append(preTestSteps, run)
}

func runPreTestSteps() error {
	for _, run := range preTestSteps {
		if err := run(); err != nil {
			return err
		}
	}
	return nil
}
