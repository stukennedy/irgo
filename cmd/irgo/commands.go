package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// runDev starts the development server with hot reload
func runDev() error {
	// Check for required tools
	if err := checkTool("air", "go install github.com/air-verse/air@latest"); err != nil {
		return err
	}
	if err := checkTool("templ", "go install github.com/a-h/templ/cmd/templ@latest"); err != nil {
		return err
	}
	if err := checkTool("entr", "brew install entr"); err != nil {
		return err
	}

	// Check if dev.sh exists (user project) or we're in framework
	if _, err := os.Stat("dev.sh"); err == nil {
		// User project - run dev.sh
		return runCommand("./dev.sh")
	}

	// Framework development - run air directly
	fmt.Println("Starting development server...")

	// Generate templ files first
	if err := runTempl(); err != nil {
		fmt.Printf("Warning: templ generate failed: %v\n", err)
	}

	return runCommand("air")
}

// runServe starts the server without file watching
func runServe() error {
	// Check if main.go exists
	if _, err := os.Stat("main.go"); err == nil {
		// User project
		return runCommand("go", "run", ".", "serve")
	}

	// Framework - run example
	if _, err := os.Stat("examples/todo/main.go"); err == nil {
		return runCommand("go", "run", "./examples/todo", "serve")
	}

	return fmt.Errorf("no main.go found - are you in an irgo project?")
}

// runBuild builds for mobile platforms
func runBuild(target string) error {
	// Check for gomobile
	if err := checkTool("gomobile", "go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"); err != nil {
		return err
	}

	// Determine module path
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}

	// Create build directory
	if err := os.MkdirAll("build", 0755); err != nil {
		return fmt.Errorf("creating build directory: %w", err)
	}

	switch target {
	case "ios":
		return buildIOS(modulePath)
	case "android":
		return buildAndroid(modulePath)
	case "all":
		if err := buildIOS(modulePath); err != nil {
			return err
		}
		return buildAndroid(modulePath)
	default:
		return fmt.Errorf("unknown build target: %s (use ios, android, or all)", target)
	}
}

func buildIOS(modulePath string) error {
	fmt.Println("Building iOS framework...")

	outPath := "build/ios/Irgo.xcframework"
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	// Remove existing framework
	os.RemoveAll(outPath)

	// Ensure go.work and gomobile setup
	if err := ensureMobileBuildSetup(); err != nil {
		return fmt.Errorf("mobile build setup failed: %w", err)
	}

	mobilePackage := modulePath + "/mobile"
	if err := runGomobileCommand("bind", "-target", "ios", "-o", outPath, mobilePackage); err != nil {
		return fmt.Errorf("gomobile bind failed: %w", err)
	}

	fmt.Printf("iOS framework built: %s\n", outPath)
	return nil
}

func buildAndroid(modulePath string) error {
	fmt.Println("Building Android AAR...")

	outPath := "build/android/irgo.aar"
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}

	// Remove existing AAR
	os.Remove(outPath)

	// Ensure go.work and gomobile setup
	if err := ensureMobileBuildSetup(); err != nil {
		return fmt.Errorf("mobile build setup failed: %w", err)
	}

	mobilePackage := modulePath + "/mobile"
	// -androidapi 24 matches the minSdk used by the Android Example project
	// (android/Example/app/build.gradle.kts). Modern NDKs reject API versions
	// below 21; gomobile's default of 16 is too old.
	//
	// -javapkg irgo renames the Java package from the Go package's name
	// ("mobile") to "irgo.mobile" so the Kotlin bridge in IrgoBridge.kt can
	// `import irgo.mobile.Mobile`.
	if err := runGomobileCommand("bind", "-target", "android", "-androidapi", "24",
		"-javapkg", "irgo", "-o", outPath, mobilePackage); err != nil {
		return fmt.Errorf("gomobile bind failed: %w", err)
	}

	fmt.Printf("Android AAR built: %s\n", outPath)

	// Copy to Example project if it exists
	exampleLibsPath := "android/Example/app/libs/irgo.aar"
	if _, err := os.Stat("android/Example"); err == nil {
		os.MkdirAll(filepath.Dir(exampleLibsPath), 0755)
		if err := copyFile(outPath, exampleLibsPath); err != nil {
			fmt.Printf("Warning: could not copy to example project: %v\n", err)
		} else {
			fmt.Printf("Copied to: %s\n", exampleLibsPath)
		}
	}

	return nil
}

// runTempl generates templ files
func runTempl() error {
	if err := checkTool("templ", "go install github.com/a-h/templ/cmd/templ@latest"); err != nil {
		return err
	}

	fmt.Println("Generating templ files...")
	return runCommand("templ", "generate")
}

// runTest runs the test suite
func runTest() error {
	fmt.Println("Running tests...")
	return runCommand("go", "test", "-v", "./...")
}

// installTools installs required development tools
func installTools() error {
	fmt.Println("Installing irgo development tools...")
	fmt.Println()

	tools := []struct {
		name string
		pkg  string
	}{
		{"templ", "github.com/a-h/templ/cmd/templ@latest"},
		{"air", "github.com/air-verse/air@latest"},
		{"gomobile", "golang.org/x/mobile/cmd/gomobile@latest"},
	}

	for _, tool := range tools {
		if _, err := exec.LookPath(tool.name); err != nil {
			fmt.Printf("Installing %s...\n", tool.name)
			cmd := exec.Command("go", "install", tool.pkg)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("  Warning: failed to install %s: %v\n", tool.name, err)
			} else {
				fmt.Printf("  %s: installed\n", tool.name)
			}
		} else {
			fmt.Printf("  %s: already installed\n", tool.name)
		}
	}

	// Initialize gomobile
	fmt.Println()
	fmt.Println("Initializing gomobile...")
	if err := runCommand("gomobile", "init"); err != nil {
		fmt.Printf("Warning: gomobile init failed: %v\n", err)
		fmt.Println("You may need to run 'gomobile init' manually after installing Android NDK")
	}

	fmt.Println()
	fmt.Println("Tools installed! You may also want to install:")
	fmt.Println("  - entr: brew install entr (for file watching)")
	fmt.Println("  - Xcode: from App Store (for iOS development)")
	fmt.Println("  - Android Studio: https://developer.android.com/studio (for Android development)")

	return nil
}

// runMobile builds and runs on mobile simulator
func runMobile(platform string, devMode bool) error {
	switch platform {
	case "ios":
		return runIOS(devMode)
	case "android":
		return runAndroid()
	default:
		return fmt.Errorf("unknown platform: %s (use ios or android)", platform)
	}
}

func runIOS(devMode bool) error {
	// Check for Xcode
	if err := checkTool("xcodebuild", "Install Xcode from the App Store"); err != nil {
		return err
	}
	if err := checkTool("xcrun", "Install Xcode Command Line Tools: xcode-select --install"); err != nil {
		return err
	}

	// Check if ios/Example project exists
	iosProjectPath := "ios/Example"
	if _, err := os.Stat(iosProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("iOS project not found at %s\n\nTo set up iOS development:\n"+
			"  1. Create an Xcode project at ios/Example/\n"+
			"  2. Add build/ios/Irgo.xcframework to the project\n"+
			"  3. Copy ios/Irgo/*.swift files to your project\n"+
			"  4. Set IrgoWebViewController as the root view controller", iosProjectPath)
	}

	// Dev server URL for simulator to connect to
	devServerURL := "http://localhost:8080"
	var devServerCmd *exec.Cmd

	if devMode {
		fmt.Println("Running in DEV MODE with hot reload...")
		fmt.Println()

		// Check for required dev tools
		if err := checkTool("air", "go install github.com/air-verse/air@latest"); err != nil {
			return err
		}

		// Still need to build the framework for the Xcode project
		modulePath, err := getModulePath()
		if err != nil {
			return fmt.Errorf("could not determine module path: %w", err)
		}

		fmt.Println("Building iOS framework...")
		if err := buildIOS(modulePath); err != nil {
			return err
		}

		// Update Info.plist to enable dev mode
		infoPlistPath := filepath.Join(iosProjectPath, "Example/Info.plist")
		if err := setDevServerInPlist(infoPlistPath, devServerURL); err != nil {
			fmt.Printf("Warning: could not set dev server in Info.plist: %v\n", err)
		}

		// Start dev server in background
		fmt.Printf("Starting dev server at %s...\n", devServerURL)
		devServerCmd = exec.Command("air")
		devServerCmd.Stdout = os.Stdout
		devServerCmd.Stderr = os.Stderr
		if err := devServerCmd.Start(); err != nil {
			return fmt.Errorf("failed to start dev server: %w", err)
		}

		// Give server time to start
		fmt.Println("Waiting for dev server to start...")
		exec.Command("sleep", "3").Run()

	} else {
		// Production mode: build the framework
		modulePath, err := getModulePath()
		if err != nil {
			return fmt.Errorf("could not determine module path: %w", err)
		}

		fmt.Println("Building iOS framework...")
		if err := buildIOS(modulePath); err != nil {
			return err
		}

		// Clear dev server from Info.plist for production builds
		infoPlistPath := filepath.Join(iosProjectPath, "Example/Info.plist")
		clearDevServerInPlist(infoPlistPath)
	}

	// Find the workspace or project
	var buildCmd []string
	// Use generic simulator destination to work with any available iPhone
	destination := "generic/platform=iOS Simulator"
	if _, err := os.Stat(filepath.Join(iosProjectPath, "Example.xcworkspace")); err == nil {
		buildCmd = []string{"xcodebuild", "-workspace", filepath.Join(iosProjectPath, "Example.xcworkspace"),
			"-scheme", "Example", "-destination", destination,
			"-derivedDataPath", "build/ios/DerivedData"}
	} else if _, err := os.Stat(filepath.Join(iosProjectPath, "Example.xcodeproj")); err == nil {
		buildCmd = []string{"xcodebuild", "-project", filepath.Join(iosProjectPath, "Example.xcodeproj"),
			"-scheme", "Example", "-destination", destination,
			"-derivedDataPath", "build/ios/DerivedData"}
	} else {
		return fmt.Errorf("no Xcode project found in %s", iosProjectPath)
	}

	fmt.Println("Building iOS app...")
	if err := runCommand(buildCmd[0], buildCmd[1:]...); err != nil {
		if devServerCmd != nil {
			devServerCmd.Process.Kill()
		}
		return fmt.Errorf("xcodebuild failed: %w", err)
	}

	// Find the built app
	appPath := "build/ios/DerivedData/Build/Products/Debug-iphonesimulator/Example.app"
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		if devServerCmd != nil {
			devServerCmd.Process.Kill()
		}
		return fmt.Errorf("built app not found at %s", appPath)
	}

	// Find an available iPhone simulator
	simulatorName := findAvailableIPhoneSimulator()
	if simulatorName == "" {
		simulatorName = "iPhone 15" // Fallback
	}

	// Boot simulator if needed
	fmt.Printf("Launching iOS Simulator (%s)...\n", simulatorName)
	runCommand("xcrun", "simctl", "boot", simulatorName) // Ignore error if already booted

	// Open Simulator app
	runCommand("open", "-a", "Simulator")

	// Install app
	fmt.Println("Installing app...")
	if err := runCommand("xcrun", "simctl", "install", "booted", appPath); err != nil {
		if devServerCmd != nil {
			devServerCmd.Process.Kill()
		}
		return fmt.Errorf("failed to install app: %w", err)
	}

	// Launch app
	fmt.Println("Launching app...")
	bundleID := "com.irgo.Example" // Default bundle ID
	if err := runCommand("xcrun", "simctl", "launch", "booted", bundleID); err != nil {
		if devServerCmd != nil {
			devServerCmd.Process.Kill()
		}
		return fmt.Errorf("failed to launch app: %w", err)
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

		// Wait for dev server to exit (user presses Ctrl+C)
		devServerCmd.Wait()
	} else {
		fmt.Println("\nApp running on iOS Simulator!")
	}

	return nil
}

// findAvailableIPhoneSimulator finds an available iPhone simulator
func findAvailableIPhoneSimulator() string {
	// Get list of available simulators
	out, err := exec.Command("xcrun", "simctl", "list", "devices", "available", "-j").Output()
	if err != nil {
		return ""
	}

	// Parse JSON to find an iPhone
	// Look for common iPhone names in priority order
	preferences := []string{"iPhone 15 Pro", "iPhone 15", "iPhone 17 Pro", "iPhone 17", "iPhone SE"}
	outStr := string(out)
	for _, name := range preferences {
		if strings.Contains(outStr, name) {
			return name
		}
	}

	return ""
}

func runAndroid() error {
	// Check for Android tools
	if err := checkTool("adb", "Install Android SDK and add platform-tools to PATH"); err != nil {
		return err
	}

	// Build the AAR first
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}

	fmt.Println("Building Android AAR...")
	if err := buildAndroid(modulePath); err != nil {
		return err
	}

	// Check if android/Example project exists
	androidProjectPath := "android/Example"
	if _, err := os.Stat(androidProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("Android project not found at %s\n\nIf this directory is missing, your scaffold may have been generated with an older irgo version. Re-run `irgo new <name>` in a temp dir and copy the android/ folder over, or grab the canonical example from github.com/stukennedy/irgo/android/Example.", androidProjectPath)
	}

	// Build with Gradle
	gradlew := filepath.Join(androidProjectPath, "gradlew")
	if _, err := os.Stat(gradlew); os.IsNotExist(err) {
		return fmt.Errorf("gradlew not found in %s", androidProjectPath)
	}

	// Gradle needs to know where the Android SDK is. If the user hasn't set
	// ANDROID_HOME / ANDROID_SDK_ROOT in their shell, write a local.properties
	// for them. local.properties is the canonical Android way to point a
	// project at the SDK and is always gitignored, so it doesn't pollute the
	// repo. Gradle reads sdk.dir from it before falling back to env vars.
	if err := ensureAndroidSDKConfigured(androidProjectPath); err != nil {
		return err
	}

	fmt.Println("Building Android app...")
	// Use "./gradlew" so os/exec resolves the path relative to cmd.Dir.
	// Passing the full "android/Example/gradlew" path here would make Go
	// look for "android/Example/android/Example/gradlew" because relative
	// Path values in exec.Cmd are joined onto Dir before lookup.
	cmd := exec.Command("./gradlew", "assembleDebug")
	cmd.Dir = androidProjectPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gradle build failed: %w", err)
	}

	// Find the built APK
	apkPath := filepath.Join(androidProjectPath, "app/build/outputs/apk/debug/app-debug.apk")
	if _, err := os.Stat(apkPath); os.IsNotExist(err) {
		return fmt.Errorf("built APK not found at %s", apkPath)
	}

	// Check for running emulator
	fmt.Println("Installing on Android device/emulator...")
	if err := runCommand("adb", "install", "-r", apkPath); err != nil {
		return fmt.Errorf("failed to install APK (is an emulator running?): %w", err)
	}

	// Launch app. Read the applicationId from the project rather than
	// hardcoding it — the scaffold templates the namespace per-project
	// (com.irgo.<ident>) so a fixed value would only work for the framework's
	// own Example.
	fmt.Println("Launching app...")
	packageName, err := readApplicationID(androidProjectPath)
	if err != nil {
		return fmt.Errorf("could not determine package name: %w", err)
	}
	activityName := ".MainActivity"
	if err := runCommand("adb", "shell", "am", "start", "-n", packageName+"/"+packageName+activityName); err != nil {
		return fmt.Errorf("failed to launch app: %w", err)
	}

	fmt.Println("\nApp running on Android!")
	return nil
}

// Helper functions

// readApplicationID parses applicationId from app/build.gradle.kts.
// This is the most reliable source of truth before the APK is built; aapt
// would also work but isn't always on PATH.
func readApplicationID(androidProjectPath string) (string, error) {
	gradlePath := filepath.Join(androidProjectPath, "app", "build.gradle.kts")
	data, err := os.ReadFile(gradlePath)
	if err != nil {
		return "", err
	}
	// Matches:  applicationId = "com.foo.bar"
	re := regexp.MustCompile(`applicationId\s*=\s*"([^"]+)"`)
	m := re.FindStringSubmatch(string(data))
	if len(m) < 2 {
		return "", fmt.Errorf("no applicationId found in %s", gradlePath)
	}
	return m[1], nil
}

// findAndroidSDK locates the Android SDK by checking the standard env vars
// then falling back to well-known install paths. Returns an empty string if
// no SDK can be found.
func findAndroidSDK() string {
	// Env vars take precedence — if the user set them, trust them.
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(env); v != "" {
			if isAndroidSDK(v) {
				return v
			}
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Well-known install locations, in order of preference per platform.
	// Android Studio defaults differ by OS.
	candidates := []string{
		filepath.Join(home, "Android", "Sdk"),                     // Linux + Android Studio
		filepath.Join(home, "Library", "Android", "sdk"),          // macOS + Android Studio
		filepath.Join(home, "AppData", "Local", "Android", "Sdk"), // Windows
		filepath.Join(home, "android-sdk"),                        // older manual installs
		"/opt/android-sdk",                                        // system-wide on Linux
		"/usr/local/share/android-sdk",                            // Homebrew on macOS
	}
	for _, c := range candidates {
		if isAndroidSDK(c) {
			return c
		}
	}
	return ""
}

// isAndroidSDK returns true if dir looks like an Android SDK root. We check
// for platform-tools and either platforms or build-tools — the bare minimum
// for Gradle to build a debug APK.
func isAndroidSDK(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "platform-tools")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "platforms")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "build-tools")); err == nil {
		return true
	}
	return false
}

// ensureAndroidSDKConfigured writes a local.properties pointing at the
// detected Android SDK, unless the project already has one. Returns a
// helpful error if no SDK can be located.
func ensureAndroidSDKConfigured(androidProjectPath string) error {
	localPropsPath := filepath.Join(androidProjectPath, "local.properties")
	if data, err := os.ReadFile(localPropsPath); err == nil {
		if strings.Contains(string(data), "sdk.dir=") {
			return nil // Already configured — respect the user's choice.
		}
	}

	sdkPath := findAndroidSDK()
	if sdkPath == "" {
		return fmt.Errorf(`Android SDK not found.

Tried:
  $ANDROID_HOME and $ANDROID_SDK_ROOT (both empty or invalid)
  ~/Android/Sdk, ~/Library/Android/sdk, ~/AppData/Local/Android/Sdk
  /opt/android-sdk, /usr/local/share/android-sdk

Install the Android SDK (via Android Studio or the command-line tools), then:
  - Set ANDROID_HOME in your shell, OR
  - Add 'sdk.dir=/path/to/sdk' to %s`, localPropsPath)
	}

	// Gradle on Windows wants forward slashes in local.properties to avoid
	// escape-character ambiguity. filepath.ToSlash handles all platforms.
	content := fmt.Sprintf("# Auto-generated by `irgo run android`. Safe to edit.\nsdk.dir=%s\n", filepath.ToSlash(sdkPath))
	if err := os.WriteFile(localPropsPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", localPropsPath, err)
	}
	fmt.Printf("Configured Android SDK: %s\n", sdkPath)
	return nil
}

func checkTool(name, installCmd string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found. Install with: %s", name, installCmd)
	}
	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func getModulePath() (string, error) {
	// Try to read from go.mod
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}

	// Parse module line
	lines := string(data)
	for _, line := range splitLines(lines) {
		if len(line) > 7 && line[:7] == "module " {
			return line[7:], nil
		}
	}

	return "", fmt.Errorf("module directive not found in go.mod")
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// ensureMobileBuildSetup ensures the go.work file and x/mobile are set up correctly
func ensureMobileBuildSetup() error {
	goVersion := getGoVersion()

	// Check if go.work exists with x/mobile
	if _, err := os.Stat("go.work"); os.IsNotExist(err) {
		// Get irgo path for replacement
		irgoPath := getIrgoPath()

		// Clone x/mobile if not already present
		mobileDir := filepath.Join(os.TempDir(), "golang-mobile")
		if _, err := os.Stat(mobileDir); os.IsNotExist(err) {
			fmt.Println("Cloning golang.org/x/mobile...")
			cmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/golang/mobile", mobileDir)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to clone x/mobile: %w", err)
			}
		}

		// Update go.mod in cloned repo to use current Go version
		mobileModPath := filepath.Join(mobileDir, "go.mod")
		if data, err := os.ReadFile(mobileModPath); err == nil {
			content := string(data)
			// Replace any go 1.x.x version with current version
			lines := splitLines(content)
			for i, line := range lines {
				if len(line) > 3 && line[:3] == "go " {
					lines[i] = "go " + goVersion
					break
				}
			}
			os.WriteFile(mobileModPath, []byte(strings.Join(lines, "\n")), 0644)
		}

		// Create go.work file
		workContent := fmt.Sprintf("go %s\n\nuse (\n\t.\n", goVersion)
		if irgoPath != "" {
			workContent += fmt.Sprintf("\t%s\n", irgoPath)
		}
		workContent += fmt.Sprintf("\t%s\n)\n", mobileDir)

		if err := os.WriteFile("go.work", []byte(workContent), 0644); err != nil {
			return fmt.Errorf("failed to create go.work: %w", err)
		}
		fmt.Println("Created go.work for mobile build")

		// Install gomobile and gobind from local source
		fmt.Println("Installing gomobile from source...")
		cmd := exec.Command("go", "install", "./cmd/gomobile")
		cmd.Dir = mobileDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install gomobile: %w", err)
		}

		cmd = exec.Command("go", "install", "./cmd/gobind")
		cmd.Dir = mobileDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install gobind: %w", err)
		}
	}

	return nil
}

// runGomobileCommand runs a gomobile command with the correct GOTOOLCHAIN
func runGomobileCommand(args ...string) error {
	goVersion := getGoVersion()

	cmd := exec.Command("gomobile", args...)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go"+goVersion)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// setDevServerInPlist adds IRGO_DEV_SERVER to Info.plist
func setDevServerInPlist(plistPath, devServerURL string) error {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return err
	}

	content := string(data)

	// Check if IRGO_DEV_SERVER already exists
	if strings.Contains(content, "IRGO_DEV_SERVER") {
		// Update existing value
		// Find and replace the string value after IRGO_DEV_SERVER key
		start := strings.Index(content, "<key>IRGO_DEV_SERVER</key>")
		if start != -1 {
			stringStart := strings.Index(content[start:], "<string>")
			stringEnd := strings.Index(content[start:], "</string>")
			if stringStart != -1 && stringEnd != -1 {
				newContent := content[:start+stringStart+8] + devServerURL + content[start+stringEnd:]
				return os.WriteFile(plistPath, []byte(newContent), 0644)
			}
		}
	}

	// Add new key-value pair before </dict>
	insertPoint := strings.LastIndex(content, "</dict>")
	if insertPoint == -1 {
		return fmt.Errorf("could not find </dict> in Info.plist")
	}

	newEntry := fmt.Sprintf("\t<key>IRGO_DEV_SERVER</key>\n\t<string>%s</string>\n", devServerURL)
	newContent := content[:insertPoint] + newEntry + content[insertPoint:]

	return os.WriteFile(plistPath, []byte(newContent), 0644)
}

// clearDevServerInPlist removes IRGO_DEV_SERVER from Info.plist
func clearDevServerInPlist(plistPath string) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return
	}

	content := string(data)

	// Find and remove IRGO_DEV_SERVER entry
	keyStart := strings.Index(content, "<key>IRGO_DEV_SERVER</key>")
	if keyStart == -1 {
		return
	}

	// Find the end of the string value
	valueEnd := strings.Index(content[keyStart:], "</string>")
	if valueEnd == -1 {
		return
	}

	// Remove the entire entry including newlines
	entryEnd := keyStart + valueEnd + len("</string>")

	// Look for trailing newline
	if entryEnd < len(content) && content[entryEnd] == '\n' {
		entryEnd++
	}

	// Look for leading whitespace/newline
	entryStart := keyStart
	for entryStart > 0 && (content[entryStart-1] == '\t' || content[entryStart-1] == ' ') {
		entryStart--
	}

	newContent := content[:entryStart] + content[entryEnd:]
	os.WriteFile(plistPath, []byte(newContent), 0644)
}
