//go:build !darwin

package desktop

// SetupMenu is a no-op on non-macOS platforms.
// On macOS, this configures the native menu bar.
func SetupMenu(appName, version string) {
	// No-op on non-macOS platforms
}
