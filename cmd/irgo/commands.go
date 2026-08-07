// What the CLI can do, declared once.
//
// This used to be three things that had to agree: a dispatch switch, a
// hand-written index, and a page of prose per command. They did not agree.
// `app build cloudflare` shipped working and undocumented; `--avd` was
// documented after it was removed; nine flags existed that no help mentioned;
// the README described commands the CLI no longer had.
//
// So the facts live here and everything else is derived. help.go renders them,
// cmd_route.go dispatches against them, and the generated README table is the
// same data again. A command that is not in this file has no help, no index
// entry and no README row — and a test fails rather than shipping any of it.
//
// The prose survives as notes. It is the part that cannot be derived, and it
// is the part worth reading: why --avd was removed, why shared state cannot
// live in a Go variable on Workers, why wrangler needs Node.
package main

// command is one noun-verb pair.
type command struct {
	summary string      // one line: the index, the README, the title
	targets []string    // platform targets, shown as <a|b|c>
	args    string      // index hint when it takes no targets
	usage   [][2]string // {form, what it does}
	flags   [][2]string // {spec, what it does}
	notes   string      // the reasoning, rendered last
}

// commands is keyed "noun verb".
var commands = map[string]command{

	// ---- project -----------------------------------------------------------

	"project new": {
		summary: "Create a project, or regenerate this one",
		args:    "<name>",
		usage: [][2]string{
			{"<name>", "Create ./<name>"},
			{".", "Generate into the current directory"},
			{"--check", "Report what regenerating would change, write nothing"},
		},
		notes: `What it writes:
  main.go, handlers/, templates/, static/   your app
  ios/, android/                            native shells
  .github/workflows/                        CI for every target
  go.mod                                    pins the CLI via a tool directive

Files that are yours are seeded once and never overwritten: go.mod, README.md,
irgo.package.toml and appicon.png. Everything else is regenerated, so fix the
template rather than the generated copy.

--check exits non-zero if regenerating would change a file. That asserts the
repo IS unmodified CLI output, which is true of example repos and not of a real
app — for those, use irgo project upgrade --check.`,
	},

	"project upgrade": {
		summary: "Take framework updates, leaving your code alone",
		args:    "[--check|--diff|--force]",
		usage: [][2]string{
			{"", "Refresh framework-owned scaffolding"},
			{"--check", "Name what an upgrade would overwrite, change nothing"},
			{"--diff", "Also show what the template holds for your files"},
			{"--force", "Overwrite your files too (destructive)"},
		},
		notes: `Framework-owned (replaced): ios/, android/, mobile/, .air.toml, .gitignore,
CLAUDE.md, AGENTS.md, and the generated workflows.
Yours (never rewritten):    main.go, handlers/, templates/, static/, README.md,
                            go.mod, irgo.package.toml, appicon.png.

Anything overwritten is copied to <file>.irgo-bak first.

--check is the CI-friendly form: it exits non-zero when a framework-owned file
has been hand-edited, i.e. when an upgrade is about to discard that edit.`,
	},

	"project pin": {
		summary: "Choose which irgo this project builds against",
		args:    "[local|release|<version>]",
		usage: [][2]string{
			{"", "Show the current pin and where it came from"},
			{"local [dir]", "Build a checkout you are editing"},
			{"release", "Track the published module"},
			{"<version>", "A published version, e.g. v0.4.0"},
			{"<owner>/<repo>@<tag>", "A fork"},
		},
		notes: "go.mod is the only pin: `go tool irgo` builds whatever it names, so there is\n" +
			`nothing installed globally to fall out of step. A pin that does not resolve
leaves go.mod untouched rather than half-written.

A fork keeps the upstream module path, so the proxy cannot serve it:
  go env -w GOPRIVATE='github.com/<owner>/*'`,
	},

	"project ci": {
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
	},

	"project clean": {
		summary: "Remove generated output",
		args:    "[--all]",
		usage: [][2]string{
			{"", "Generated code and build output"},
			{"--all", "Also the scaffolded native shells"},
		},
		notes: `Removes _templ.go, static/css/output.css, build/, tmp/ and dist/. With --all,
ios/Example and android/Example go too; they are scaffolded again on the next
build, which takes a gomobile rebuild.

Nothing you wrote is touched.`,
	},

	"project config": {
		summary: "Show or set a setting (signing, stores, version)",
		args:    "[<key>] [<value>]",
		usage: [][2]string{
			{"", "Every setting, its value, and where it came from"},
			{"<key>", "One setting"},
			{"<key> <value>", "Set it"},
		},
		notes: `Settings live in irgo.package.toml. Precedence: environment variable, then
irgo.package.local.toml (gitignored), then irgo.package.toml.

Secrets belong in the local file or the environment — irgo.package.toml is
committed. Values you have to discover rather than type, such as which signing
teams exist, are reported by irgo tools doctor.`,
	},

	"project assets": {
		summary: "Regenerate templ + Tailwind (builds do this already)",
		usage:   [][2]string{{"", "Write _templ.go and static/css/output.css"}},
		notes: "Both are gitignored and compiled into the binary, so a plain `go build` needs\n" +
			`this first; every irgo build and run does it for you.

templ and the Tailwind standalone binary are installed on demand. There is no
Node, npm or package.json.`,
	},

	"project test": {
		summary: "Run the tests",
		usage:   [][2]string{{"", "Regenerate assets, then go test ./..."}},
		notes: `Assets first, so tests that render templates see the current ones rather than
whatever was last on disk.`,
	},

	// ---- app ---------------------------------------------------------------

	"app build": {
		summary: "Build it",
		targets: []string{"ios", "android", "desktop", "cloudflare", "all"},
		usage: [][2]string{
			{"ios", "Device framework (Irgo.xcframework)"},
			{"ios --sim", "Simulator build"},
			{"ios --device", "Build and sign for a USB device"},
			{"android", "AAR for the native shell"},
			{"desktop", "This host"},
			{"desktop <goos>", "linux, darwin or windows"},
			{"cloudflare", "A Cloudflare Worker (WASM) in build/worker"},
			{"all", "Everything this host can produce"},
		},
		flags: [][2]string{
			{"--sim, -s", "Simulator rather than device (iOS)"},
			{"--device, -D", "A real device (iOS)"},
			{"--team <id>", "Apple Team ID to sign with"},
		},
		notes: `Cloudflare compiles the same router to WebAssembly and serves it from a
Worker, SSE included. Shared state cannot live in a Go variable there — every
request gets a fresh runtime — so keep it in a KV, D1 or Durable Object
binding. Per-connection state within one SSE stream is fine.

Toolchains install themselves: the Android SDK, NDK, JDK and gomobile are
provisioned on first use. Cross-building is limited by the host — macOS can
produce macOS and Windows desktop binaries, Linux only Linux. Ask irgo tools
doctor what this machine can do.

On Linux, ios builds the device slice (ios-arm64) with the xtool Darwin SDK
(https://xtool.sh) instead of Xcode, and copies the framework into the
ios/App SwiftPM project. --sim and --device still need macOS.`,
	},

	"app run": {
		summary: "Build and launch it",
		targets: []string{"ios", "android", "desktop"},
		usage: [][2]string{
			{"ios", "iOS Simulator"},
			{"ios --device", "A USB-connected iPhone"},
			{"android", "Android emulator"},
			{"desktop", "Native desktop window"},
		},
		flags: [][2]string{
			{"--dev, -d", "Hot reload: serve from the dev server on :8080 and reload on save"},
			{"--device, -D", "A real iPhone rather than the Simulator"},
			{"--team <id>", "Apple Team ID to sign with"},
			{"--built, -b", "Desktop: launch the existing build rather than rebuilding"},
		},
		notes: `Android uses whatever device or emulator is already connected, and only boots
its own when nothing is. To run a particular AVD, start it first — there is no
flag to disagree with what is actually attached.

On Linux, ios deploys to a USB/network-connected physical device via xtool
(https://xtool.sh) instead of the Simulator. With --dev the device connects
to the dev server over your LAN; IRGO_DEV_SERVER=http://<host>:8080 overrides
the URL.`,
	},

	"app deploy": {
		summary: "Build the Worker and put it live",
		targets: []string{"cloudflare"},
		usage:   [][2]string{{"cloudflare", "Build the Worker and deploy it"}},
		notes: `Builds first, so what goes live is what the current source produces.

Cloudflare needs wrangler, which is a Node program — and Cloudflare does not
support the bun runtime. irgo downloads its own Node into ~/.irgo rather than
asking you to install one, and irgo tools remove takes it away again. A working
node already on PATH is used instead.

Credentials:
  CLOUDFLARE_API_TOKEN   required in CI
                         From a terminal, wrangler opens a browser instead.

The Worker's name and any bindings come from wrangler.toml, which is yours
after irgo seeds it.`,
	},

	"app package": {
		summary: "Store artifacts",
		targets: []string{"ios", "android", "macos", "windows"},
		usage: [][2]string{
			{"ios", "Signed .ipa"},
			{"android", "Signed .aab"},
			{"macos", "Signed .app, notarized; --dmg for a disk image"},
			{"windows", ".msix"},
			{"setup --check", "What each store needs, and what is missing"},
		},
		flags: [][2]string{
			{"--team <id>", "iOS: Apple Team ID"},
			{"--export-method <m>", "iOS: app-store, ad-hoc or development"},
			{"--keystore <path>", "Android: signing keystore"},
			{"--keystore-pass <s>", "Android: keystore password"},
			{"--key-alias <s>", "Android: key alias"},
			{"--key-pass <s>", "Android: key password"},
			{"--identity <cert>", "macOS: Developer ID Application certificate"},
			{"--notarize", "macOS: notarize the build"},
			{"--apple-id <email>", "macOS: Apple ID for notarization"},
			{"--password <s>", "macOS: app-specific password"},
			{"--dmg", "macOS: also produce a disk image"},
			{"--publisher <dn>", "Windows: publisher DN"},
			{"--cert <pfx>", "Windows: code-signing certificate"},
			{"--cert-pass <s>", "Windows: certificate password"},
			{"--version <v>", "Override common.version"},
			{"--icon <path>", "Override the source icon"},
			{"--output, -o", "Where to write the artifact"},
		},
		notes: `Every signing setting can be given on the command line as well as in
irgo.package.toml, which is how CI supplies secrets without writing them to
disk. Missing credentials are reported before the build rather than at the end
of it, and assets are regenerated first so a package cannot ship a stale
stylesheet.`,
	},

	"app install": {
		summary: "Install a build — no rebuild",
		targets: []string{"ios", "android", "desktop"},
		usage: [][2]string{
			{"ios", "Onto the running Simulator"},
			{"android", "Onto the connected device or emulator"},
			{"desktop", "Into /Applications (macOS)"},
		},
		notes: `Installs the existing artifact and does not rebuild. Build first if there is
nothing there yet.`,
	},

	"app remove": {
		summary: "Uninstall it again",
		targets: []string{"ios", "android", "desktop"},
		usage: [][2]string{
			{"ios", "From the Simulator"},
			{"android", "From the device or emulator"},
			{"desktop", "From /Applications"},
		},
		notes: `The exact inverse of irgo app install. Every install irgo performs can be
undone by irgo, so nothing it puts on a machine has to be hunted down by hand.`,
	},

	"app reviews": {
		summary: "Monitor store reviews",
		targets: []string{"ios", "mac", "android"},
		usage: [][2]string{
			{"<store>", "Fetch reviews"},
			{"ios --new", "Only ones you have not seen"},
			{"ios --reply <id> --text \"...\"", "Reply to one"},
		},
		flags: [][2]string{
			{"--limit <n>", "How many to fetch"},
			{"--new", "Only ones you have not seen"},
			{"--reply <id>", "Reply to one review"},
			{"--text \"...\"", "The reply body"},
		},
		notes: `Needs the store credentials in irgo.package.toml under [reviews] — see
irgo project config.`,
	},

	// ---- tools -------------------------------------------------------------

	"tools doctor": {
		summary: "What this host can build; --fix repairs it",
		args:    "[android] [--fix|--strict]",
		usage: [][2]string{
			{"", "Every target, and what is missing for the rest"},
			{"android", "The Android toolchain in detail"},
			{"--fix", "Install what is missing"},
			{"--strict", "Exit non-zero if anything is missing (CI)"},
		},
		notes: `Reports the Go toolchain, templ and Tailwind, Xcode and its signing teams, and
the Android SDK/NDK/JDK with the emulator and AVDs.`,
	},

	"tools install": {
		summary: "Provision what builds need",
		args:    "[android] [--emulator]",
		usage: [][2]string{
			{"", "Everything this host can use"},
			{"android", "SDK, NDK, JDK 17 and gomobile"},
			{"android --emulator", "Also a system image and an AVD"},
			{"android --avd <name>", "Name that AVD (default \"irgo\")"},
		},
		flags: [][2]string{
			{"--emulator, -e", "Also install a system image and an AVD"},
			{"--avd <name>", "Name the AVD"},
		},
		notes: `Everything lands under ~/.irgo or the Android SDK home — no system package
manager, on any OS. Running it again installs only what is missing.

You rarely need this: a build provisions what it needs. It exists so a large
download can be done deliberately rather than in the middle of a build.`,
	},

	"tools remove": {
		summary: "Undo it — shows what it will delete, and asks",
		args:    "[android] [--all] [--yes]",
		usage: [][2]string{
			{"", "What irgo installed for this host"},
			{"android", "The Android toolchain"},
			{"--all", "Everything, including the SDK and AVDs"},
		},
		flags: [][2]string{
			{"--all", "Everything irgo installed, including the SDK and AVDs"},
			{"--keep-jdk", "Leave the managed JDK in place"},
			{"--yes, -y", "Do not ask"},
		},
		notes: `Shows what it will delete, with sizes, and asks first. Outside a terminal it
refuses rather than assuming yes.

Only what irgo installed is removed: each install leaves a marker, and anything
without one is left alone, so a toolchain you set up yourself survives.`,
	},

	// ---- server ------------------------------------------------------------

	"server dev": {
		summary: "Web server with hot reload",
		usage:   [][2]string{{"", "Serve on :8080 and rebuild on save"}},
		notes: `Rebuilds templ, Tailwind and the Go binary as you save. From an Android
emulator the same server is http://10.0.2.2:8080, which is what
irgo app run android --dev connects to.

air is installed on demand.`,
	},

	"server serve": {
		summary: "Web server without file watching",
		usage:   [][2]string{{"", "Serve on :8080"}},
		notes: `Regenerates assets once at startup and then leaves them alone — for checking a
build, or running the web target.`,
	},
}
