// Installing an already-built app.
//
// `irgo app run` builds, installs and launches in one step, which is right while
// developing and wrong when you want to check the artifact itself: whether the
// signed bundle installs, whether the packaged app behaves like it will for a
// user. Doing that meant copying files by hand, which is the fiddling the CLI
// exists to remove — and it left `app` with a remove and no install.
//
// Nothing is built here. Whatever exists is installed, and the packaged
// artifact wins over the development one, because that is the thing that
// actually ships.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runAppInstall installs the built app for a platform.
func runAppInstall(platform string) error {
	switch platform {
	case "ios":
		return installIOSApp()
	case "android":
		return installAndroidApp()
	case "desktop":
		return installDesktopApp()
	case "all":
		var errs []string
		for _, p := range []string{"ios", "android", "desktop"} {
			if err := runAppInstall(p); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return nil
	default:
		return fmt.Errorf("unknown platform: %s (use ios, android, desktop, or all)", platform)
	}
}

// iosSimulatorApp is where `irgo app build ios --sim` leaves its bundle.
const iosSimulatorApp = "build/ios/DerivedData/Build/Products/Debug-iphonesimulator/Example.app"

func installIOSApp() error {
	if err := requireMacOS("iOS install"); err != nil {
		return err
	}
	if !pathExists(iosSimulatorApp) {
		return fmt.Errorf("no Simulator app at %s — build it first: irgo app build ios --sim", iosSimulatorApp)
	}
	fmt.Printf("  ios: installing %s on the booted simulator\n", iosSimulatorApp)
	if err := runCommand("xcrun", "simctl", "install", "booted", iosSimulatorApp); err != nil {
		return fmt.Errorf("simctl install failed (is a simulator booted? try: irgo app run ios): %w", err)
	}
	fmt.Printf("  ios: installed %s\n", iosBundleID)
	return nil
}

func installAndroidApp() error {
	adb := adbBin()
	if adb == "" {
		return fmt.Errorf("adb not found — install the toolchain first: irgo tools install android")
	}
	apk := filepath.Join("android/Example", "app/build/outputs/apk/debug/app-debug.apk")
	if !pathExists(apk) {
		return fmt.Errorf("no APK at %s — build it first: irgo app run android", apk)
	}
	fmt.Printf("  android: installing %s\n", apk)
	// -r replaces an existing install rather than failing on a conflict.
	if err := runCommand(adb, "install", "-r", apk); err != nil {
		return fmt.Errorf("adb install failed (is a device or emulator attached?): %w", err)
	}
	return nil
}

// installDesktopApp puts the app where a user would have it, so it can be
// launched and uninstalled like a real install rather than run from build/.
func installDesktopApp() error {
	modulePath, err := getModulePath()
	if err != nil {
		return err
	}
	name := filepath.Base(modulePath)

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("desktop install is macOS-only; on %s run the binary from build/desktop/%s",
			runtime.GOOS, runtime.GOOS)
	}

	// The packaged bundle is signed and entitled — the one that ships — so it
	// is preferred when present.
	src := filepath.Join("dist/macos", name+".app")
	origin := "packaged"
	if !pathExists(src) {
		src = filepath.Join("build/desktop/macos", name+".app")
		origin = "development"
	}
	if !pathExists(src) {
		return fmt.Errorf("no macOS app found — build one first: irgo app build desktop (or irgo app package macos)")
	}

	dst := "/Applications/" + name + ".app"
	if pathExists(dst) {
		// Replacing rather than merging: a stale file left inside an old
		// bundle is exactly the sort of thing that produces a confusing
		// runtime failure.
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("removing the existing %s: %w", dst, err)
		}
	}
	fmt.Printf("  desktop: installing the %s build to %s\n", origin, dst)
	if err := copyDir(src, dst); err != nil {
		return fmt.Errorf("installing to /Applications (permission?): %w", err)
	}
	fmt.Printf("  desktop: installed %s\n", dst)
	fmt.Println("  run it:  open " + dst)
	return nil
}

// appBundleID is the identifier `irgo app run` installs under.
func appBundleID() (string, error) {
	modulePath, err := getModulePath()
	if err != nil {
		return "", fmt.Errorf("could not determine module path: %w", err)
	}
	return bundleIDFromModulePath(modulePath), nil
}

// runAppUninstall removes the installed app for a platform. Absence is success:
// the point is to end up with the app not installed, and reporting failure for
// an app that was never there just adds noise to a cleanup script.
func runAppUninstall(platform string) error {
	switch platform {
	case "ios":
		return uninstallIOSApp()
	case "android":
		return uninstallAndroidApp()
	case "desktop":
		return uninstallDesktopApp()
	case "all":
		var errs []string
		for _, p := range []string{"ios", "android", "desktop"} {
			if err := runAppUninstall(p); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return nil
	default:
		return fmt.Errorf("unknown platform: %s (use ios, android, desktop, or all)", platform)
	}
}

func uninstallIOSApp() error {
	if runtime.GOOS != "darwin" {
		fmt.Println("  ios: skipped (simulators only exist on macOS)")
		return nil
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		fmt.Println("  ios: skipped (xcrun not found)")
		return nil
	}
	// The example app installs under this identifier; the Xcode project sets it
	// independently of the Go module path.
	const bundleID = "com.irgo.Example"
	out, err := exec.Command("xcrun", "simctl", "uninstall", "booted", bundleID).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "No devices are booted") {
			fmt.Println("  ios: no booted simulator — nothing to remove")
			return nil
		}
		// simctl reports a missing app as an error; that is the desired state.
		fmt.Printf("  ios: not installed on the booted simulator\n")
		return nil
	}
	fmt.Printf("  ios: removed %s from the booted simulator\n", bundleID)
	return nil
}

func uninstallAndroidApp() error {
	adb := adbBin()
	if adb == "" {
		fmt.Println("  android: skipped (adb not found — install the toolchain first)")
		return nil
	}
	const pkg = "com.irgo.example"
	out, err := exec.Command(adb, "uninstall", pkg).CombinedOutput()
	text := strings.TrimSpace(string(out))
	switch {
	case err != nil && strings.Contains(text, "no devices"):
		fmt.Println("  android: no device or emulator attached — nothing to remove")
	case strings.Contains(text, "Success"):
		fmt.Printf("  android: removed %s\n", pkg)
	default:
		fmt.Printf("  android: not installed (%s)\n", firstLine(text))
	}
	return nil
}

// uninstallDesktopApp removes a copy installed into the system Applications
// directory. Build output under build/ is `irgo project clean`'s job, not this one.
func uninstallDesktopApp() error {
	if runtime.GOOS != "darwin" {
		fmt.Printf("  desktop: nothing to remove on %s (the app runs from build/ — use irgo project clean)\n", runtime.GOOS)
		return nil
	}
	modulePath, err := getModulePath()
	if err != nil {
		return err
	}
	name := baseName(modulePath)
	found := false
	for _, p := range []string{
		"/Applications/" + name + ".app",
		filepath.Join(homeDir(), "Applications", name+".app"),
	} {
		if pathExists(p) {
			fmt.Printf("  desktop: removing %s\n", p)
			if err := removeAllPath(p); err != nil {
				return err
			}
			found = true
		}
	}
	// Packaged output is `irgo project clean`'s business, but say it is there —
	// otherwise "uninstalled" is confusing when the app is still on disk.
	if pathExists(filepath.Join("dist/macos", name+".app")) {
		fmt.Printf("  desktop: dist/macos/%s.app remains (packaged output — irgo project clean removes it)\n", name)
	}
	if !found {
		fmt.Println("  desktop: not installed in /Applications")
	}
	return nil
}

// The app verbs that take a platform. Flag parsing lives here rather than in
// the router so the router stays a table of what exists.

func runAppBuild(target string, args []string) error {
	if target == "desktop" {
		platform := ""
		for _, a := range args[1:] {
			if !strings.HasPrefix(a, "-") {
				platform = a
				break
			}
		}
		return buildDesktop(platform)
	}
	team := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--team" {
			team = args[i+1]
		}
	}
	return runBuild(target,
		hasFlag(args, "--sim", "-s"),
		hasFlag(args, "--device", "-D"),
		team)
}

func runAppRun(target string, args []string) error {
	// Flags are read from the whole argument list rather than from everything
	// after the first token: the target is a bare word and every flag is
	// dashed, so there is nothing to strip, and stripping blindly ate the
	// first flag whenever the target was written after it.
	rest := args
	devMode := hasFlag(rest, "--dev", "-d")
	if target == "desktop" {
		return runDesktop(devMode, hasFlag(rest, "--built", "-b"))
	}
	if target == "ios" && hasFlag(rest, "--device", "-D") {
		// On non-macOS hosts a device is the only thing `run ios` can
		// target, and xtool (not devicectl) does the deploying.
		if !isDarwinHost() {
			return runIOSXtool(devMode)
		}
		team := ""
		for i := 0; i < len(rest)-1; i++ {
			if rest[i] == "--team" {
				team = rest[i+1]
			}
		}
		return runIOSDevice(team)
	}
	return runMobile(target, devMode)
}

func runAppPackage(args []string) error {
	return packageCommand(args)
}
