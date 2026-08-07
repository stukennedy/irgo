// iOS on non-macOS hosts, via xtool.
//
// iOS development on Linux (and other non-macOS hosts) is supported via
// xtool (https://xtool.sh) instead of Xcode.
//
// xtool's `xtool setup` installs a "Darwin Swift SDK" — an artifact bundle
// extracted from Xcode.xip that contains the real iPhoneOS/iPhoneSimulator/
// MacOSX SDKs plus a small toolset (ld64.lld, libtool, dsymutil). Combined
// with a Swift toolchain (whose clang can target arm64-apple-ios), that is
// everything needed to cross-compile the gomobile framework and the SwiftPM
// app from Linux:
//
//  1. gomobile bind -target ios/arm64  → build/ios/Irgo.xcframework
//     gomobile assumes Xcode tooling (xcrun / xcodebuild / lipo), so we
//     generate small shim scripts that answer the handful of invocations
//     gomobile actually makes, backed by the xtool SDK + Swift clang.
//  2. Apple `libtool -static` rewrites the framework binary so it gains a
//     __.SYMDEF table of contents. Go's c-archive output has no TOC, and
//     ld64.lld silently skips archive members without one.
//  3. The scaffolded SwiftPM project at ios/App (xtool.yml + Package.swift)
//     consumes the xcframework as a binary target. `xtool dev run` builds,
//     signs, and installs the app on a USB/network-connected device.
//
// Only the device slice (ios-arm64) is built: there is no iOS Simulator on
// Linux, xtool deploys to physical devices.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// isDarwinHost reports whether the CLI is running on macOS. Used to decide
// between the Xcode toolchain (macOS) and the xtool toolchain (everything
// else) for iOS builds.
func isDarwinHost() bool {
	return runtime.GOOS == "darwin"
}

// appleToolchain describes the cross-compilation toolchain assembled from
// the xtool Darwin SDK and a Swift toolchain.
type appleToolchain struct {
	// artifactBundle is the path to darwin.artifactbundle (the xtool SDK).
	artifactBundle string
	// libtool is Apple libtool from the SDK toolset (used to add a TOC).
	libtool string
	// clang is a clang capable of targeting arm64-apple-ios.
	clang string
}

// platformsDir returns <bundle>/Developer/Platforms.
func (tc *appleToolchain) platformsDir() string {
	return filepath.Join(tc.artifactBundle, "Developer", "Platforms")
}

// sdkPath maps an Xcode SDK name (iphoneos, iphonesimulator, macosx) to the
// corresponding SDK root inside the Darwin artifact bundle.
func (tc *appleToolchain) sdkPath(sdkName string) string {
	switch sdkName {
	case "iphoneos":
		return filepath.Join(tc.platformsDir(), "iPhoneOS.platform", "Developer", "SDKs", "iPhoneOS.sdk")
	case "iphonesimulator":
		return filepath.Join(tc.platformsDir(), "iPhoneSimulator.platform", "Developer", "SDKs", "iPhoneSimulator.sdk")
	case "macosx":
		return filepath.Join(tc.platformsDir(), "MacOSX.platform", "Developer", "SDKs", "MacOSX.sdk")
	}
	return ""
}

// clangTargetForSDK maps an Xcode SDK name to the clang -target triple used
// by the per-SDK clang wrapper scripts. Only the iphoneos wrapper is ever
// invoked (we build the device slice only); the others exist because
// gomobile's env setup probes every Apple SDK at startup.
func clangTargetForSDK(sdkName string) string {
	switch sdkName {
	case "iphoneos":
		return "arm64-apple-ios"
	case "iphonesimulator":
		return "arm64-apple-ios-simulator"
	case "macosx":
		return "arm64-apple-macos"
	}
	return ""
}

// findDarwinArtifactBundle locates the xtool Darwin SDK. The
// IRGO_DARWIN_SDK env var overrides the default search path.
func findDarwinArtifactBundle() (string, error) {
	if p := os.Getenv("IRGO_DARWIN_SDK"); p != "" {
		if _, err := os.Stat(filepath.Join(p, "Developer", "Platforms")); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("IRGO_DARWIN_SDK=%s does not look like a darwin.artifactbundle (missing Developer/Platforms)", p)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	bundle := filepath.Join(home, ".swiftpm", "swift-sdks", "darwin.artifactbundle")
	if _, err := os.Stat(filepath.Join(bundle, "Developer", "Platforms")); err != nil {
		return "", fmt.Errorf(`darwin Swift SDK not found at %s

iOS development on Linux requires xtool (https://xtool.sh):
  1. Install xtool and a Swift 6.1+ toolchain
  2. Run 'xtool setup' to install the Darwin SDK (needs Xcode.xip)

If your SDK lives elsewhere, set IRGO_DARWIN_SDK to its path`, bundle)
	}
	return bundle, nil
}

// findAppleClang locates a clang that can cross-compile for Apple targets.
// The clang bundled with the Swift toolchain is preferred (it matches the
// Darwin SDK), falling back to whatever clang is on PATH.
func findAppleClang() (string, error) {
	// Prefer the clang that ships next to the swift binary.
	if swiftPath, err := exec.LookPath("swift"); err == nil {
		if resolved, err := filepath.EvalSymlinks(swiftPath); err == nil {
			candidate := filepath.Join(filepath.Dir(resolved), "clang")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
		// swiftly-style shims: clang lives in the same bin dir as swift.
		candidate := filepath.Join(filepath.Dir(swiftPath), "clang")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if clang, err := exec.LookPath("clang"); err == nil {
		return clang, nil
	}
	return "", fmt.Errorf("clang not found. Install a Swift toolchain (https://swift.org/install) — its clang can target iOS")
}

// findAppleToolchain assembles the full cross toolchain or returns a
// descriptive error explaining what is missing.
func findAppleToolchain() (*appleToolchain, error) {
	bundle, err := findDarwinArtifactBundle()
	if err != nil {
		return nil, err
	}

	tc := &appleToolchain{artifactBundle: bundle}

	if sdk := tc.sdkPath("iphoneos"); true {
		if _, err := os.Stat(sdk); err != nil {
			return nil, fmt.Errorf("iPhoneOS.sdk not found at %s — re-run 'xtool setup'", sdk)
		}
	}

	libtool := filepath.Join(bundle, "toolset", "bin", "libtool")
	if _, err := os.Stat(libtool); err != nil {
		return nil, fmt.Errorf("libtool not found at %s — re-run 'xtool setup'", libtool)
	}
	tc.libtool = libtool

	clang, err := findAppleClang()
	if err != nil {
		return nil, err
	}
	tc.clang = clang

	return tc, nil
}

// appleShimsDir returns the directory where the xcrun/xcodebuild/lipo shim
// scripts are generated. Lives in the user cache so paths stay stable.
func appleShimsDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "irgo-apple-shims"), nil
	}
	return filepath.Join(cache, "irgo", "apple-shims"), nil
}

// writeAppleShims (re)generates the shim scripts that satisfy gomobile's
// expectations of an Xcode toolchain:
//
//	xcrun --sdk <sdk> --find clang   → per-SDK clang wrapper
//	xcrun --sdk <sdk> --show-sdk-path→ SDK root inside the artifact bundle
//	xcrun xcodebuild -version        → fake Xcode version (capability check)
//	xcrun lipo IN -create -o OUT     → single-slice copy (device-only build)
//	xcodebuild -create-xcframework   → manual .xcframework assembly
func writeAppleShims(dir string, tc *appleToolchain) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	writeScript := func(name, content string) error {
		return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755)
	}

	// Per-SDK clang wrappers.
	for _, sdk := range []string{"iphoneos", "iphonesimulator", "macosx"} {
		script := fmt.Sprintf(`#!/bin/sh
# Generated by irgo. clang wrapper targeting the %s SDK.
exec %q -target %s -isysroot %q "$@"
`, sdk, tc.clang, clangTargetForSDK(sdk), tc.sdkPath(sdk))
		if err := writeScript("clang-"+sdk, script); err != nil {
			return err
		}
	}

	xcrun := fmt.Sprintf(`#!/bin/sh
# Generated by irgo. Minimal xcrun shim backed by the xtool Darwin SDK.
DIR=%q
case "$1" in
  --sdk)
    sdk="$2"; shift 2
    case "$1" in
      --find)
        if [ "$2" = "clang" ] || [ "$2" = "clang++" ]; then
          echo "$DIR/clang-$sdk"; exit 0
        fi
        echo "xcrun shim: cannot find $2" >&2; exit 1 ;;
      --show-sdk-path)
        case "$sdk" in
          iphoneos) echo %q ;;
          iphonesimulator) echo %q ;;
          macosx) echo %q ;;
          *) echo "xcrun shim: unknown sdk $sdk" >&2; exit 1 ;;
        esac
        exit 0 ;;
    esac
    ;;
  lipo) shift; exec "$DIR/lipo" "$@" ;;
  xcodebuild) shift; exec "$DIR/xcodebuild" "$@" ;;
esac
echo "xcrun shim: unsupported invocation: $*" >&2
exit 1
`, dir, tc.sdkPath("iphoneos"), tc.sdkPath("iphonesimulator"), tc.sdkPath("macosx"))
	if err := writeScript("xcrun", xcrun); err != nil {
		return err
	}

	lipo := `#!/bin/sh
# Generated by irgo. Single-slice lipo shim: with one input archive,
# "lipo IN -create -o OUT" is just a copy. Fat binaries are never needed on
# Linux because only the ios-arm64 device slice is built.
inputs=""; out=""; expect=0
for a in "$@"; do
  if [ $expect -eq 1 ]; then out="$a"; expect=0; continue; fi
  case "$a" in
    -create) ;;
    -o|-output) expect=1 ;;
    -*) echo "lipo shim: unsupported flag $a" >&2; exit 1 ;;
    *) inputs="$inputs $a" ;;
  esac
done
set -- $inputs
if [ -z "$out" ]; then echo "lipo shim: missing -o/-output" >&2; exit 1; fi
if [ $# -eq 1 ]; then exec cp "$1" "$out"; fi
echo "lipo shim: fat binaries are not supported on Linux (got $# inputs)" >&2
exit 1
`
	if err := writeScript("lipo", lipo); err != nil {
		return err
	}

	xcodebuild := `#!/bin/sh
# Generated by irgo. Handles the two xcodebuild invocations gomobile
# makes: "-version" (capability probe) and "-create-xcframework".
if [ "$1" = "-version" ]; then
  echo "Xcode 16.0"
  echo "Build version 16A242d"
  exit 0
fi
if [ "$1" != "-create-xcframework" ]; then
  echo "xcodebuild shim: unsupported invocation: $*" >&2
  exit 1
fi
shift
fws=""; out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -framework) fws="$fws $2"; shift 2 ;;
    -output) out="$2"; shift 2 ;;
    *) echo "xcodebuild shim: unsupported arg $1" >&2; exit 1 ;;
  esac
done
if [ -z "$out" ]; then echo "xcodebuild shim: missing -output" >&2; exit 1; fi
rm -rf "$out"
mkdir -p "$out"
libs=""
for fw in $fws; do
  case "$fw" in
    */iphoneos/*) id="ios-arm64"; platform="ios"; archs="<string>arm64</string>"; variant="" ;;
    */iphonesimulator/*) id="ios-arm64-simulator"; platform="ios"; archs="<string>arm64</string>"; variant="
			<key>SupportedPlatformVariant</key>
			<string>simulator</string>" ;;
    *) echo "xcodebuild shim: cannot infer platform from framework path $fw" >&2; exit 1 ;;
  esac
  name="$(basename "$fw")"
  mkdir -p "$out/$id"
  cp -R "$fw" "$out/$id/$name"
  base="$(basename "$fw" .framework)"
  libs="$libs
		<dict>
			<key>BinaryPath</key>
			<string>$name/$base</string>
			<key>LibraryIdentifier</key>
			<string>$id</string>
			<key>LibraryPath</key>
			<string>$name</string>
			<key>SupportedArchitectures</key>
			<array>$archs</array>
			<key>SupportedPlatform</key>
			<string>$platform</string>$variant
		</dict>"
done
cat > "$out/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>AvailableLibraries</key>
	<array>$libs
	</array>
	<key>CFBundlePackageType</key>
	<string>XFWK</string>
	<key>XCFrameworkFormatVersion</key>
	<string>1.0</string>
</dict>
</plist>
PLIST
exit 0
`
	return writeScript("xcodebuild", xcodebuild)
}

// buildIOSXtool builds build/ios/Irgo.xcframework on a non-macOS host
// using the xtool Darwin SDK. Mirrors buildIOS (app_ios_build.go) but:
//   - only the ios-arm64 device slice is built (no simulator on Linux)
//   - gomobile's Xcode tooling expectations are satisfied by shim scripts
//   - the resulting static archive gets a TOC added via Apple libtool so
//     ld64.lld links it correctly
func buildIOSXtool(modulePath string) error {
	fmt.Println("Building iOS framework (Linux + xtool toolchain)...")

	tc, err := findAppleToolchain()
	if err != nil {
		return err
	}

	outPath := "build/ios/Irgo.xcframework"
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(outPath)

	// Ensure go.work and gomobile setup
	if err := ensureMobileBuildSetup(); err != nil {
		return fmt.Errorf("mobile build setup failed: %w", err)
	}

	shimDir, err := appleShimsDir()
	if err != nil {
		return err
	}
	if err := writeAppleShims(shimDir, tc); err != nil {
		return fmt.Errorf("writing Xcode tool shims: %w", err)
	}

	// Only the device slice: xtool deploys to physical devices, and there is
	// no iOS Simulator on Linux. Not runGomobileCommand: the shim directory
	// must be first on PATH so gomobile resolves xcrun/xcodebuild/lipo here.
	mobilePackage := modulePath + "/mobile"
	cmd := exec.Command("gomobile", "bind", "-target", "ios/arm64", "-o", outPath, mobilePackage)
	cmd.Env = append(os.Environ(),
		"GOTOOLCHAIN=go"+runningGoVersion(),
		"PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gomobile bind failed: %w", err)
	}

	// Go's c-archive output has no __.SYMDEF table of contents; ld64.lld
	// silently skips archive members without one, producing "undefined
	// symbol" errors at app link time. Apple libtool rebuilds the archive
	// with a proper TOC.
	fwBinary := filepath.Join(outPath, "ios-arm64", "Irgo.framework", "Irgo")
	tocTmp := fwBinary + ".toc"
	tocCmd := exec.Command(tc.libtool, "-static", "-o", tocTmp, fwBinary)
	tocCmd.Stdout = os.Stdout
	tocCmd.Stderr = os.Stderr
	if err := tocCmd.Run(); err != nil {
		return fmt.Errorf("adding archive table of contents with libtool: %w", err)
	}
	if err := os.Rename(tocTmp, fwBinary); err != nil {
		return err
	}
	writeArtifactStamp("build/ios")

	fmt.Printf("iOS framework built: %s\n", outPath)

	// Keep the SwiftPM app's copy in sync (it consumes the xcframework as a
	// binary target rooted inside the package).
	if err := syncXcframeworkToApp(outPath); err != nil {
		fmt.Printf("Warning: could not copy framework to ios/App: %v\n", err)
	}

	return nil
}

// syncXcframeworkToApp copies the built xcframework into the SwiftPM app
// package (ios/App/Irgo.xcframework) if that project exists. SwiftPM
// requires binary target paths to live inside the package directory.
// copyDir is declared in app_desktop_build.go and reused here.
func syncXcframeworkToApp(outPath string) error {
	appDir := filepath.Join("ios", "App")
	if _, err := os.Stat(filepath.Join(appDir, "Package.swift")); err != nil {
		return nil // No SwiftPM app scaffold — nothing to sync.
	}
	dest := filepath.Join(appDir, "Irgo.xcframework")
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := copyDir(outPath, dest); err != nil {
		return err
	}
	fmt.Printf("Copied to: %s\n", dest)
	return nil
}

// scaffoldIOSApp writes the SwiftPM/xtool app project (ios/App) into the
// current directory from the embedded templates. Missing-only and idempotent,
// like scaffoldExamples: ios/Example is the Xcode shell macOS builds, ios/App
// is the SwiftPM shell xtool builds on Linux.
func scaffoldIOSApp() error {
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}
	projectName := filepath.Base(modulePath)
	if projectName == "." || projectName == "" || projectName == "/" {
		wd, _ := os.Getwd()
		projectName = filepath.Base(wd)
	}
	if projectName == "." || projectName == "" {
		projectName = "irgo"
	}
	fmt.Println("Ensuring SwiftPM iOS app project (ios/App)...")
	return scaffoldExampleDir("ios/App", projectName, modulePath)
}

// runIOSXtool implements `irgo app run ios [--dev]` on non-macOS hosts:
// build the gomobile framework, build/sign/install the SwiftPM app with
// xtool, then (best-effort) launch it on the connected device.
func runIOSXtool(devMode bool) error {
	if err := checkTool("xtool", "see https://xtool.sh — install xtool, then run 'xtool setup'"); err != nil {
		return err
	}

	// Ensure the SwiftPM app shell exists (scaffolded from the embedded
	// templates when missing — devs and CI never hand-copy it).
	if err := scaffoldIOSApp(); err != nil {
		return fmt.Errorf("iOS app scaffold failed: %w", err)
	}
	appDir := filepath.Join("ios", "App")

	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}

	infoPlistPath := filepath.Join(appDir, "Info.plist")
	var devServerCmd *exec.Cmd
	var devServerURL string

	if devMode {
		fmt.Println("Running in DEV MODE with hot reload...")
		fmt.Println()

		if err := ensureGoTool("air"); err != nil {
			return err
		}

		// A physical device cannot reach "localhost" on this machine — use
		// the LAN IP. (ATS does not apply to raw IP literals, so plain HTTP
		// works without Info.plist exceptions.) Set IRGO_DEV_SERVER to
		// override, e.g. when the device reaches this machine via VPN/Tailscale.
		if env := os.Getenv("IRGO_DEV_SERVER"); env != "" {
			devServerURL = env
		} else {
			ip := lanIPv4()
			if ip == "" {
				return fmt.Errorf("could not determine this machine's LAN IP address (needed so the device can reach the dev server). Set IRGO_DEV_SERVER=http://<host>:8080 to override")
			}
			devServerURL = "http://" + ip + ":8080"
		}

		// The SwiftPM package links the xcframework as a binary target, so it
		// must exist even in dev mode — but a fresh gomobile build is only
		// needed when missing or built by a different irgo version.
		if !artifactUpToDate("build/ios/Irgo.xcframework", "build/ios") {
			fmt.Println("Building iOS framework (missing or built by another irgo version)...")
			if err := buildIOS(modulePath); err != nil {
				return err
			}
		} else {
			fmt.Println("Using existing build/ios/Irgo.xcframework (delete it to force a rebuild)")
			if err := syncXcframeworkToApp("build/ios/Irgo.xcframework"); err != nil {
				fmt.Printf("Warning: could not copy framework to ios/App: %v\n", err)
			}
		}

		if err := setDevServerInPlist(infoPlistPath, devServerURL); err != nil {
			fmt.Printf("Warning: could not set dev server in Info.plist: %v\n", err)
		}

		fmt.Printf("Starting dev server at %s...\n", devServerURL)
		fmt.Println("(Your device must be on the same network as this machine.)")
		devServerCmd = exec.Command("air")
		devServerCmd.Stdout = os.Stdout
		devServerCmd.Stderr = os.Stderr
		if err := devServerCmd.Start(); err != nil {
			return fmt.Errorf("failed to start dev server: %w", err)
		}
	} else {
		if err := buildIOS(modulePath); err != nil {
			return err
		}
		clearDevServerInPlist(infoPlistPath)
	}

	killDevServer := func() {
		if devServerCmd != nil && devServerCmd.Process != nil {
			_ = devServerCmd.Process.Kill()
		}
	}

	// Build, sign, and install on the connected device. xtool waits for a
	// device to be connected if none is found.
	fmt.Println("Building and installing iOS app with xtool...")
	xtoolRun := exec.Command("xtool", "dev", "run")
	xtoolRun.Dir = appDir
	xtoolRun.Stdout = os.Stdout
	xtoolRun.Stderr = os.Stderr
	xtoolRun.Stdin = os.Stdin
	if err := xtoolRun.Run(); err != nil {
		killDevServer()
		return fmt.Errorf("xtool dev run failed: %w", err)
	}

	// Best-effort launch. xtool rewrites the bundle ID during provisioning
	// to "XTL-<identity>.<bundleID>"; launching over debugserver can also
	// fail if the device is locked, so fall back to a tap-the-icon hint.
	launched := launchOnDeviceViaXtool(appDir)
	if !launched {
		fmt.Println("App installed. Tap the app icon on your device to launch it.")
		fmt.Println("(Automatic launch needs the device unlocked and, on iOS <17, a")
		fmt.Println(" mounted developer disk image — see 'ideviceimagemounter'.)")
	}

	if devMode {
		fmt.Println()
		fmt.Println("===========================================")
		fmt.Println("iOS app running in DEV MODE with hot reload!")
		fmt.Printf("Dev server: %s\n", devServerURL)
		fmt.Println("Edit your Go code and see changes instantly.")
		fmt.Println("Press Ctrl+C to stop.")
		fmt.Println("===========================================")
		fmt.Println()
		_ = devServerCmd.Wait()
	} else {
		fmt.Println("\nApp deployed to iOS device!")
	}

	return nil
}

// launchOnDeviceViaXtool tries to launch the freshly installed app. Returns
// true if the launch command succeeded.
func launchOnDeviceViaXtool(appDir string) bool {
	bundleID, err := readXtoolBundleID(filepath.Join(appDir, "xtool.yml"))
	if err != nil || bundleID == "" {
		return false
	}
	prefix := xtoolBundleIDPrefix()
	if prefix == "" {
		return false
	}
	fmt.Println("Launching app...")
	cmd := exec.Command("xtool", "launch", prefix+bundleID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run() == nil
}

// readXtoolBundleID extracts the bundleID value from an xtool.yml file.
func readXtoolBundleID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?m)^bundleID:\s*(\S+)\s*$`)
	m := re.FindStringSubmatch(string(data))
	if len(m) < 2 {
		return "", fmt.Errorf("no bundleID found in %s", path)
	}
	return m[1], nil
}

// xtoolBundleIDPrefix computes the provisioning prefix xtool applies to
// bundle IDs when signing ("XTL-<identity>." where identity is the first
// dash-separated segment of the signing identity, uppercased — the Team ID
// for Apple ID auth). Returns "" if it cannot be determined.
func xtoolBundleIDPrefix() string {
	out, err := exec.Command("xtool", "auth", "status").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	re := regexp.MustCompile(`Team ID:\s*(\S+)`)
	m := re.FindSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	teamID := string(m[1])
	identity := strings.ToUpper(strings.SplitN(teamID, "-", 2)[0])
	return "XTL-" + identity + "."
}

// sanitizeBundleIdent converts a project name into a fragment safe for use
// in an Apple bundle ID. Unlike sanitizeIdentifier (which targets Java/Kotlin
// packages and preserves underscores), Apple's provisioning API rejects
// underscores and most punctuation in App ID names, so anything that is not
// a letter or digit is stripped, and a leading digit gets an "app" prefix.
//
// Examples: "my-app" -> "myapp", "my_app" -> "myapp", "123game" -> "app123game".
func sanitizeBundleIdent(name string) string {
	if name == "" {
		return "app"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "app" + result
	}
	if result == "" {
		return "app"
	}
	return result
}

// lanIPv4 returns this machine's primary outbound IPv4 address, or "" if it
// cannot be determined. Used so a physical iOS device on the same network
// can reach the dev server.
func lanIPv4() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer func() { _ = conn.Close() }()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	return addr.IP.String()
}
