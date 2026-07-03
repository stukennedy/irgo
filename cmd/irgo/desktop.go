package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runDesktop builds and runs a desktop app
func runDesktop(devMode bool) error {
	fmt.Println("Starting desktop app...")

	args := []string{"run", "-tags", "desktop", "."}
	if devMode {
		args = append(args, "--dev")
	}

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// buildDesktop builds desktop app for target platform
func buildDesktop(target string) error {
	if target == "" {
		target = runtime.GOOS
	}

	fmt.Printf("Building desktop app for %s...\n", target)

	// Generate templ files first
	if err := runTempl(); err != nil {
		fmt.Printf("Warning: templ generate failed: %v\n", err)
	}

	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("could not determine module path: %w", err)
	}

	switch target {
	case "darwin", "macos":
		return buildDesktopMacOS(modulePath)
	case "windows":
		return buildDesktopWindows(modulePath)
	case "linux":
		return buildDesktopLinux(modulePath)
	default:
		return fmt.Errorf("unsupported desktop platform: %s (use darwin, windows, or linux)", target)
	}
}

func buildDesktopMacOS(modulePath string) error {
	appName := filepath.Base(modulePath)
	outDir := "build/desktop/macos"
	appBundle := filepath.Join(outDir, appName+".app")

	// Create .app bundle structure
	if err := os.MkdirAll(filepath.Join(appBundle, "Contents", "MacOS"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(appBundle, "Contents", "Resources"), 0755); err != nil {
		return err
	}

	// Build the binary with CGO enabled (required for webview)
	binaryPath := filepath.Join(appBundle, "Contents", "MacOS", appName)
	cmd := exec.Command("go", "build", "-tags", "desktop", "-o", binaryPath, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	// Copy static assets to Resources
	if _, err := os.Stat("static"); err == nil {
		if err := copyDir("static", filepath.Join(appBundle, "Contents", "Resources", "static")); err != nil {
			fmt.Printf("Warning: could not copy static assets: %v\n", err)
		}
	}

	// Generate Info.plist
	plistContent := generateMacOSPlist(appName, bundleIDFromModulePath(modulePath))
	plistPath := filepath.Join(appBundle, "Contents", "Info.plist")
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("could not write Info.plist: %w", err)
	}

	fmt.Printf("macOS app built: %s\n", appBundle)
	return nil
}

func buildDesktopWindows(modulePath string) error {
	appName := filepath.Base(modulePath)
	outDir := "build/desktop/windows"

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	binaryPath := filepath.Join(outDir, appName+".exe")
	cmd := exec.Command("go", "build",
		"-tags", "desktop",
		"-ldflags", "-H windowsgui", // Hide console window
		"-o", binaryPath,
		".",
	)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	// Copy static assets
	if _, err := os.Stat("static"); err == nil {
		if err := copyDir("static", filepath.Join(outDir, "static")); err != nil {
			fmt.Printf("Warning: could not copy static assets: %v\n", err)
		}
	}

	fmt.Printf("Windows app built: %s\n", binaryPath)
	return nil
}

func buildDesktopLinux(modulePath string) error {
	appName := filepath.Base(modulePath)
	outDir := "build/desktop/linux"

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	binaryPath := filepath.Join(outDir, appName)
	cmd := exec.Command("go", "build",
		"-tags", "desktop",
		"-o", binaryPath,
		".",
	)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	// Copy static assets
	if _, err := os.Stat("static"); err == nil {
		if err := copyDir("static", filepath.Join(outDir, "static")); err != nil {
			fmt.Printf("Warning: could not copy static assets: %v\n", err)
		}
	}

	fmt.Printf("Linux app built: %s\n", binaryPath)
	return nil
}

// bundleIDFromModulePath converts a Go module path into a valid reverse-DNS
// style CFBundleIdentifier. A raw module path like "github.com/user/app" is
// invalid (contains slashes); it becomes "com.github.user.app". A bare module
// name like "myapp" becomes "com.irgo.myapp". Characters outside
// [A-Za-z0-9.-] are stripped.
func bundleIDFromModulePath(modulePath string) string {
	segments := strings.Split(modulePath, "/")

	var parts []string
	if len(segments) > 1 && strings.Contains(segments[0], ".") {
		// First segment is a host name (e.g. github.com) - reverse it
		host := strings.Split(segments[0], ".")
		for i, j := 0, len(host)-1; i < j; i, j = i+1, j-1 {
			host[i], host[j] = host[j], host[i]
		}
		parts = append(parts, host...)
		parts = append(parts, segments[1:]...)
	} else if len(segments) > 1 {
		parts = segments
	} else {
		// Bare module name like "myapp"
		parts = append([]string{"com", "irgo"}, segments...)
	}

	// Strip invalid characters from each part and drop empty parts
	sanitized := make([]string, 0, len(parts))
	for _, part := range parts {
		var b strings.Builder
		for _, r := range part {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') || r == '-' {
				b.WriteRune(r)
			}
		}
		if b.Len() > 0 {
			sanitized = append(sanitized, b.String())
		}
	}

	if len(sanitized) == 0 {
		return "com.irgo.app"
	}
	return strings.Join(sanitized, ".")
}

func generateMacOSPlist(appName, bundleID string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>%s</string>
    <key>CFBundleIdentifier</key>
    <string>%s</string>
    <key>CFBundleName</key>
    <string>%s</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>`, appName, bundleID, appName)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// hasFlag checks if any of the given flags are present in args
func hasFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag {
				return true
			}
		}
	}
	return false
}
