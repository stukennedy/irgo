package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	return fmt.Errorf("no main.go found - are you in a gohtmx project?")
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

	outPath := "build/ios/Gohtmx.xcframework"
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

	outPath := "build/android/gohtmx.aar"
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
	if err := runGomobileCommand("bind", "-target", "android", "-o", outPath, mobilePackage); err != nil {
		return fmt.Errorf("gomobile bind failed: %w", err)
	}

	fmt.Printf("Android AAR built: %s\n", outPath)

	// Copy to Example project if it exists
	exampleLibsPath := "android/Example/app/libs/gohtmx.aar"
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
	fmt.Println("Installing gohtmx development tools...")
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
func runMobile(platform string) error {
	switch platform {
	case "ios":
		return runIOS()
	case "android":
		return runAndroid()
	default:
		return fmt.Errorf("unknown platform: %s (use ios or android)", platform)
	}
}

func runIOS() error {
	// Check for Xcode
	if err := checkTool("xcodebuild", "Install Xcode from the App Store"); err != nil {
		return err
	}
	if err := checkTool("xcrun", "Install Xcode Command Line Tools: xcode-select --install"); err != nil {
		return err
	}

	// Build the framework first
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}

	fmt.Println("Building iOS framework...")
	if err := buildIOS(modulePath); err != nil {
		return err
	}

	// Check if ios/Example project exists
	iosProjectPath := "ios/Example"
	if _, err := os.Stat(iosProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("iOS project not found at %s\n\nTo set up iOS development:\n"+
			"  1. Create an Xcode project at ios/Example/\n"+
			"  2. Add build/ios/Gohtmx.xcframework to the project\n"+
			"  3. Copy ios/GoHTMX/*.swift files to your project\n"+
			"  4. Set GoHTMXWebViewController as the root view controller", iosProjectPath)
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
		return fmt.Errorf("xcodebuild failed: %w", err)
	}

	// Find the built app
	appPath := "build/ios/DerivedData/Build/Products/Debug-iphonesimulator/Example.app"
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
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
		return fmt.Errorf("failed to install app: %w", err)
	}

	// Launch app
	fmt.Println("Launching app...")
	bundleID := "com.gohtmx.Example" // Default bundle ID
	if err := runCommand("xcrun", "simctl", "launch", "booted", bundleID); err != nil {
		return fmt.Errorf("failed to launch app: %w", err)
	}

	fmt.Println("\nApp running on iOS Simulator!")
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
		return fmt.Errorf("Android project not found at %s\n\nTo set up Android development:\n"+
			"  1. Create an Android Studio project at android/Example/\n"+
			"  2. Copy build/android/gohtmx.aar to app/libs/\n"+
			"  3. Add implementation files('libs/gohtmx.aar') to build.gradle\n"+
			"  4. Copy android/app/src/main/kotlin/com/gohtmx/*.kt to your project", androidProjectPath)
	}

	// Build with Gradle
	gradlew := filepath.Join(androidProjectPath, "gradlew")
	if _, err := os.Stat(gradlew); os.IsNotExist(err) {
		return fmt.Errorf("gradlew not found in %s", androidProjectPath)
	}

	fmt.Println("Building Android app...")
	cmd := exec.Command(gradlew, "assembleDebug")
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

	// Launch app
	fmt.Println("Launching app...")
	packageName := "com.gohtmx.example"
	activityName := ".MainActivity"
	if err := runCommand("adb", "shell", "am", "start", "-n", packageName+"/"+packageName+activityName); err != nil {
		return fmt.Errorf("failed to launch app: %w", err)
	}

	fmt.Println("\nApp running on Android!")
	return nil
}

// Helper functions

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
		// Get gohtmx path for replacement
		gohtmxPath := getGoHTMXPath()

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
		if gohtmxPath != "" {
			workContent += fmt.Sprintf("\t%s\n", gohtmxPath)
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

