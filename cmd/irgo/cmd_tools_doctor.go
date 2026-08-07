// Host capability reporting: what this machine can build, what it cannot, and
// why. The CLI is the authority on this — not documentation, which goes stale
// and cannot see the machine it is describing.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// capability is one build target and this host's verdict on it.
type capability struct {
	target string
	// state is one of: ready (buildable now), fixable (buildable once
	// something installs — irgo does that automatically), blocked (this OS
	// cannot produce it at all).
	state string
	note  string
}

const (
	capReady   = "ready"
	capFixable = "auto-installs"
	// capAction is for a target that this OS supports but that needs a manual
	// step irgo cannot take — signing credentials, or a toggle on a device.
	// Distinct from capFixable, which irgo handles on the next build, and from
	// capBlocked, which nothing can fix here.
	capAction  = "NEEDS SETUP"
	capBlocked = "NOT ON THIS OS"
)

// hostCapabilities evaluates every target against the current host. Blocked
// entries are the ones that matter most: they can never succeed here, no
// matter what gets installed, so a dev needs to know before trying.
func hostCapabilities() []capability {
	var caps []capability
	has := func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

	caps = append(caps, capability{"web", capReady, "go build — works everywhere"})

	// Desktop: only Windows cross-compiles, and only from macOS.
	buildable := map[string]bool{}
	for _, t := range desktopTargetsForHost() {
		buildable[t] = true
	}
	switch {
	case buildable["darwin"]:
		note := "native"
		if !has("clang") {
			note = "needs Xcode Command Line Tools: xcode-select --install"
		}
		caps = append(caps, capability{"desktop (macOS)", capReady, note})
	default:
		caps = append(caps, capability{"desktop (macOS)", capBlocked,
			"requires macOS (Xcode) — cannot cross-compile from " + runtime.GOOS})
	}
	if buildable["windows"] {
		state, note := capReady, "native"
		if runtime.GOOS == "darwin" {
			note = "cross-compile via mingw-w64"
			if !has("x86_64-w64-mingw32-gcc") {
				state = capFixable
				note = "cross-compile — irgo installs mingw-w64 on first build"
			}
		}
		caps = append(caps, capability{"desktop (Windows)", state, note})
	} else {
		caps = append(caps, capability{"desktop (Windows)", capBlocked,
			"buildable from Windows, or macOS with mingw-w64 — not " + runtime.GOOS})
	}
	if buildable["linux"] {
		state, note := capReady, "native"
		if exec.Command("pkg-config", "--exists", "webkit2gtk-4.0").Run() != nil {
			state, note = capFixable, "irgo installs GTK3 + WebKit2GTK on first build"
		}
		caps = append(caps, capability{"desktop (Linux)", state, note})
	} else {
		caps = append(caps, capability{"desktop (Linux)", capBlocked,
			"requires Linux (GTK3 + WebKit2GTK) — cannot cross-compile from " + runtime.GOOS})
	}

	// iOS: on macOS Xcode is the one thing irgo cannot install. Elsewhere,
	// device builds work through the xtool Darwin SDK (https://xtool.sh);
	// only the Simulator is truly macOS-bound.
	if runtime.GOOS == "darwin" {
		state, note := capReady, "Xcode present"
		if !has("xcodebuild") {
			state, note = capBlocked, "install Xcode from the App Store (irgo cannot install it)"
		}
		devState, devNote := iosDeviceCapability(state)
		caps = append(caps,
			capability{"ios (framework)", state, note},
			capability{"ios (simulator app)", state, note},
			capability{"ios (device/App Store)", devState, devNote},
		)
	} else {
		// Framework builds need only the Darwin SDK toolchain — buildIOSXtool
		// never invokes the xtool binary itself. Deployment to a device
		// additionally needs xtool, so the two capabilities are reported
		// independently: a host without xtool can still build frameworks.
		fwState, fwNote := capAction,
			"install a Swift toolchain and the xtool Darwin SDK ('xtool setup', https://xtool.sh)"
		devState, devNote := fwState, fwNote
		if _, err := findAppleToolchain(); err == nil {
			fwState, fwNote = capReady, "xtool Darwin SDK present"
			if has("xtool") {
				devState, devNote = capReady, "xtool Darwin SDK present"
			} else {
				devState, devNote = capAction, "Darwin SDK present — install xtool (https://xtool.sh) to deploy"
			}
		}
		caps = append(caps,
			capability{"ios (framework)", fwState, fwNote},
			capability{"ios (simulator app)", capBlocked,
				"requires macOS (Xcode) — no iOS Simulator on " + runtime.GOOS},
			capability{"ios (device via xtool)", devState, devNote},
		)
	}

	// Packaging: what can actually be shipped from here. Each mirrors the gate
	// in runPackage, so the report cannot drift from the real behaviour.
	mac := runtime.GOOS == "darwin"
	win := runtime.GOOS == "windows"
	pkg := func(target, note, blockedNote string, ok bool) capability {
		if ok {
			return capability{target, capReady, note}
		}
		return capability{target, capBlocked, blockedNote}
	}
	caps = append(caps,
		pkg("package ios (.ipa)", "needs a signing team — irgo app package setup ios",
			"requires macOS (Xcode + codesign) — run on a Mac or macos-latest in CI", mac),
		pkg("package macos (.app/.dmg)", "notarization needs an Apple ID — irgo app package setup macos",
			"requires macOS (codesign/notarytool/hdiutil) — run on a Mac or macos-latest in CI", mac),
		pkg("package windows (.msix)", "needs the Windows SDK (MakeAppx/signtool)",
			"requires Windows (MakeAppx/signtool) — run on Windows or windows-latest in CI", win),
		capability{"package android (.aab)", capFixable,
			"builds anywhere — signing needs a keystore: irgo app package setup android"},
	)

	// Android builds anywhere; only the emulator has host limits.
	caps = append(caps, capability{"android (AAR/APK)", capFixable,
		"irgo installs the JDK, SDK and NDK on first build — Android Studio not needed"})
	if err := ensureEmulatorSupported(); err != nil {
		caps = append(caps, capability{"android (emulator)", capBlocked, firstLine(err.Error())})
	} else {
		caps = append(caps, capability{"android (emulator)", capFixable,
			"irgo installs the emulator and creates the AVD on first run"})
	}

	return caps
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// doctorHost prints the capability report. Exit status stays zero: a target
// this OS cannot build is a fact about the machine, not a failure.
func doctorHost(strict bool) error {
	fmt.Printf("irgo %s — what this machine can build\n\n", version)
	fmt.Printf("Host: %s/%s\n\n", runtime.GOOS, runtime.GOARCH)

	caps := hostCapabilities()
	w := 0
	for _, c := range caps {
		if len(c.target) > w {
			w = len(c.target)
		}
	}
	for _, c := range caps {
		fmt.Printf("  %-*s  %-15s  %s\n", w, c.target, c.state, c.note)
	}

	var blocked, action []string
	for _, c := range caps {
		switch c.state {
		case capBlocked:
			blocked = append(blocked, c.target)
		case capAction:
			action = append(action, c.target)
		}
	}
	if len(action) > 0 {
		fmt.Println()
		fmt.Printf("Needs a manual step: %s\n", strings.Join(action, ", "))
		fmt.Println("These work on this OS, but irgo cannot supply credentials or")
		fmt.Println("toggle settings on a device for you — see the note on each line.")
	}
	fmt.Println()
	if len(blocked) == 0 {
		fmt.Println("Every target is buildable here.")
	} else {
		fmt.Printf("Not possible on %s: %s\n", runtime.GOOS, strings.Join(blocked, ", "))
		fmt.Println("Build those on a matching host or in CI — the repo's workflow covers")
		fmt.Println("Linux, macOS and Windows. `irgo app build all` skips what it cannot do.")
	}
	drift := checkPinDrift()

	printToolVersions()

	printTeamDetail()
	printXcodeDetail()
	printAndroidDetail()

	fmt.Println()
	fmt.Println("Store config:     irgo app package setup --check")
	fmt.Println("Toolchain detail: irgo tools doctor android")
	if strict && drift {
		return fmt.Errorf("CLI pin drift: IRGO_REPLACE and go.mod disagree")
	}
	return nil
}

// checkPinDrift reports when the CLI pin in the environment disagrees with the
// replace directive in go.mod. Those two must match: IRGO_REPLACE selects the
// CLI, and `irgo project new` writes it into go.mod. When they drift, builds silently
// run against a different CLI than the project expects — which is exactly how
// a stale pin hides for several releases.
func checkPinDrift() bool {
	want := strings.TrimSpace(os.Getenv("IRGO_REPLACE"))
	if want == "" {
		return false
	}
	want = strings.Replace(want, "@", " ", 1)
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "replace github.com/stukennedy/irgo") {
			continue
		}
		got := strings.TrimSpace(line[strings.Index(line, "=>")+2:])
		if got != want {
			fmt.Println()
			fmt.Println("WARNING: CLI pin drift")
			fmt.Printf("  IRGO_REPLACE : %s\n", want)
			fmt.Printf("  go.mod       : %s\n", got)
			fmt.Println("  These must match. Regenerate the app (irgo project new <name>) or fix IRGO_REPLACE.")
			return true
		}
		return false
	}
	return false
}

// iosDeviceNote reports the two prerequisites a physical-device run needs that
// a simulator run does not: a signing identity, and a device that is attached
// with Developer Mode enabled. Both are manual, so surfacing them here saves a
// full build that fails at the last step.
func iosDeviceCapability(xcodeState string) (state, note string) {
	if xcodeState == capBlocked {
		return capBlocked, "install Xcode from the App Store (irgo cannot install it)"
	}
	var parts []string
	needsAction := false

	if teams := xcodeTeams(); len(teams) > 0 {
		if cur, src, terr := resolveIOSTeam(""); terr == nil {
			parts = append(parts, fmt.Sprintf("team %s (from %s)", cur, src))
		} else {
			parts = append(parts, fmt.Sprintf("%d teams, none selected — irgo ios team <ID>", len(teams)))
			needsAction = true
		}
	} else if hasSigningIdentity() {
		parts = append(parts, "signing identity present")
	} else {
		parts = append(parts, "no Apple ID in Xcode (Settings → Accounts; a free one works)")
		needsAction = true
	}

	// A device only matters for `irgo app run ios --device`; packaging an .ipa
	// needs none, so its absence is not itself a blocker.
	if devs, derr := listIOSDevices(); derr == nil && len(devs) > 0 {
		d := devs[0]
		for _, c := range devs {
			if c.connected {
				d = c
				break
			}
		}
		switch {
		case !d.connected:
			// Do not report Developer Mode here: devicectl serves the last
			// known value for a paired-but-absent device, which is stale.
			parts = append(parts, d.name+" paired but NOT CONNECTED (plug in by USB, unlock)")
			needsAction = true
		case d.devModeOn:
			parts = append(parts, d.name+" connected, Developer Mode on")
		default:
			parts = append(parts, d.name+" connected but Developer Mode OFF (Settings → Privacy & Security)")
			needsAction = true
		}
	} else {
		parts = append(parts, "no device attached (only needed for --device)")
	}

	if needsAction {
		return capAction, strings.Join(parts, "; ")
	}
	return capReady, strings.Join(parts, "; ")
}

// printXcodeDetail reports the Xcode install itself, which is separate from the
// Apple account inside it. Each line here is a real failure people hit, and
// none of them announce themselves clearly when a build breaks.
func printXcodeDetail() {
	if runtime.GOOS != "darwin" {
		return
	}
	if _, err := exec.LookPath("xcodebuild"); err != nil {
		return // already reported as blocked above
	}
	fmt.Println()
	fmt.Println("Xcode:")

	// A Mac can have the Command Line Tools selected instead of Xcode, in which
	// case xcodebuild exists but cannot build an app — a confusing failure.
	sel := "unknown"
	if out, err := exec.Command("xcode-select", "-p").Output(); err == nil {
		sel = strings.TrimSpace(string(out))
	}
	if strings.Contains(sel, "CommandLineTools") {
		fmt.Printf("  selected      %s\n", sel)
		fmt.Println("                ^ this is the Command Line Tools, not Xcode.")
		fmt.Println("                  Fix: sudo xcode-select -s /Applications/Xcode.app")
	} else {
		fmt.Printf("  selected      %s\n", sel)
	}

	ver := "unknown"
	if out, err := exec.Command("xcodebuild", "-version").Output(); err == nil {
		ver = firstLine(strings.TrimSpace(string(out)))
	}
	fmt.Printf("  version       %s\n", ver)
	if major := xcodeMajor(ver); major > 0 && major < 15 {
		fmt.Println("                ^ running on a physical device needs Xcode 15+")
		fmt.Println("                  (devicectl); the simulator still works.")
	}

	// An unaccepted licence fails every build with a message about agreeing to
	// terms, which is easy to miss inside a long log.
	if err := exec.Command("xcodebuild", "-checkFirstLaunchStatus").Run(); err != nil {
		fmt.Println("  first launch  INCOMPLETE — run: sudo xcodebuild -runFirstLaunch")
		fmt.Println("                (accepts the licence and installs components)")
	} else {
		fmt.Println("  first launch  complete (licence accepted)")
	}

	n := 0
	if out, err := exec.Command("xcrun", "simctl", "list", "runtimes").Output(); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			if strings.Contains(l, "iOS") {
				n++
			}
		}
	}
	if n == 0 {
		fmt.Println("  simulators    none installed — Xcode → Settings → Components")
	} else {
		fmt.Printf("  simulators    %d iOS runtime(s)\n", n)
	}
}

// xcodeMajor extracts the major version from "Xcode 26.6". Returns 0 when the
// string is not in that shape.
func xcodeMajor(ver string) int {
	f := strings.Fields(ver)
	if len(f) < 2 {
		return 0
	}
	num := f[1]
	if i := strings.Index(num, "."); i > 0 {
		num = num[:i]
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0
	}
	return n
}

// printTeamDetail lists every development team, so choosing between them is a
// visible decision rather than a guess about which one irgo picked.
func printTeamDetail() {
	teams := xcodeTeams()
	if len(teams) == 0 {
		return
	}
	cur, src, _ := resolveIOSTeam("")
	fmt.Println()
	fmt.Println("Development teams:")
	fmt.Print(formatTeams(teams, cur))
	if cur != "" {
		fmt.Printf("  in use: %s (from %s)\n", cur, src)
	}
	if len(teams) > 1 {
		fmt.Println("  select: irgo ios team <TEAM_ID>")
	}
}

// printAndroidDetail mirrors the Xcode section for the Android toolchain.
//
// Unlike Xcode, none of this is a prerequisite: builds provision it on first
// use. So a missing SDK is reported as "provisioned on first build" rather
// than as a fault — the section exists to answer "what is already here and
// what will be fetched", not to list errors.
func printAndroidDetail() {
	sdk := androidHome()
	fmt.Println()
	fmt.Println("Android:")

	if fi, err := os.Stat(sdk); err == nil && fi.IsDir() {
		fmt.Printf("  SDK           %s\n", sdk)
	} else {
		fmt.Printf("  SDK           not installed — fetched into %s on first build\n", sdk)
		fmt.Println("                Android Studio is not involved")
		return
	}

	if jdkHome, ok := detectJDK17(false); ok {
		where := jdkHome
		if where == "" {
			where = "java on PATH"
		} else if strings.Contains(where, managedJDKRel) {
			where += " (managed by irgo)"
		}
		fmt.Printf("  JDK 17        %s\n", where)
	} else {
		// Gradle 8.2 / AGP 8.2 fail on JDK 21+, which is what a machine
		// usually has, so this is worth naming rather than leaving to a
		// confusing Gradle error later.
		fmt.Println("  JDK 17        not found — fetched into ~/.irgo/jdks on first build")
	}

	ndk := filepath.Join(sdk, "ndk", pinNDK)
	if fi, err := os.Stat(ndk); err == nil && fi.IsDir() {
		fmt.Printf("  NDK           %s\n", pinNDK)
	} else {
		fmt.Printf("  NDK           %s not installed — fetched on first build\n", pinNDK)
	}

	emu := filepath.Join(sdk, "emulator", "emulator")
	if runtime.GOOS == "windows" {
		emu += ".exe"
	}
	switch {
	case !pathExists(emu):
		fmt.Println("  emulator      not installed — only needed for `irgo app run android`")
	default:
		avd := "none"
		if avdmgr := locateAvdmanager(sdk); avdmgr != "" {
			if out, err := exec.Command(avdmgr, "list", "avd").CombinedOutput(); err == nil {
				if strings.Contains(string(out), "Name:") {
					avd = "present"
				}
			}
		}
		fmt.Printf("  emulator      installed, AVD %s\n", avd)
	}
	fmt.Println("  detail        irgo tools doctor android")
}
