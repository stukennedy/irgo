package desktop

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// StaticFS returns a filesystem for static files.
// In development mode (devPath exists on disk), serves from filesystem.
// In production mode, serves from the embedded filesystem.
func StaticFS(embedded fs.FS, devPath string) http.FileSystem {
	// Check if running in dev mode (source files exist on disk)
	if _, err := os.Stat(devPath); err == nil {
		return http.Dir(devPath)
	}

	// Production: use embedded filesystem
	return http.FS(embedded)
}

// FindResourcePath finds the path to bundled resources.
// Handles platform-specific app bundle locations.
func FindResourcePath() string {
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)

		// macOS: Check for .app bundle Resources directory
		// App.app/Contents/MacOS/binary -> App.app/Contents/Resources
		if runtime.GOOS == "darwin" {
			resourcesDir := filepath.Join(exeDir, "..", "Resources")
			if _, err := os.Stat(resourcesDir); err == nil {
				return resourcesDir
			}
		}

		// Windows/Linux: Check for resources directory next to executable
		resourcesDir := filepath.Join(exeDir, "resources")
		if _, err := os.Stat(resourcesDir); err == nil {
			return resourcesDir
		}

		// Fallback to static directory next to executable
		staticDir := filepath.Join(exeDir, "static")
		if _, err := os.Stat(staticDir); err == nil {
			return exeDir
		}

		return exeDir
	}

	return "."
}

// FindStaticDir finds the static files directory, checking multiple locations.
// Returns the first valid path found, or "static" as fallback.
func FindStaticDir() string {
	// Check current directory first (development)
	if _, err := os.Stat("static"); err == nil {
		return "static"
	}

	// Check relative to executable
	resourcePath := FindResourcePath()

	staticInResources := filepath.Join(resourcePath, "static")
	if _, err := os.Stat(staticInResources); err == nil {
		return staticInResources
	}

	// Fallback
	return "static"
}
