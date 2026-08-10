// Small helpers shared across commands.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

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

// copyFile copies src to dst, preserving permissions.
//
// The mode matters: a hardcoded 0644 strips the execute bit from any binary
// copied through here, and the result is an .app bundle that launchd refuses
// to spawn with "Launchd job spawn failed" — a failure that says nothing about
// permissions and only shows up after packaging.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, serr := os.Stat(src); serr == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return err
	}
	// WriteFile only applies mode when it creates the file, so set it
	// explicitly for the overwrite case.
	return os.Chmod(dst, mode)
}

// toolBinDirs lists the directories a `go install` may have placed a binary in.
// The resolved GOBIN and the default $GOPATH/bin differ when GOBIN was empty at
// install time, so a tool can be in either — both must be searched on removal.
func toolBinDirs() []string {
	dirs := []string{gobinDir()}
	if out, err := exec.Command(goBin(), "env", "GOPATH").Output(); err == nil {
		if gp := strings.TrimSpace(string(out)); gp != "" {
			if p := filepath.Join(gp, "bin"); p != dirs[0] {
				dirs = append(dirs, p)
			}
		}
	}
	return dirs
}

// exeName appends the platform executable suffix.
func exeName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}

// removeTool deletes a go-installed binary from every candidate bin dir,
// reporting each path removed. Returns whether anything was found.
func removeTool(tool string, verbose bool) bool {
	found := false
	for _, dir := range toolBinDirs() {
		p := filepath.Join(dir, exeName(tool))
		if _, err := os.Stat(p); err == nil {
			if err := os.Remove(p); err == nil {
				if verbose {
					fmt.Printf("  %s: removed (%s)\n", tool, p)
				}
				found = true
			}
		}
	}
	return found
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// runHint tells the caller how to run or install what was just produced.
// Building an artifact and then having to work out how to launch it is a gap
// every dev hits; the command that made it knows the answer.
func runHint(lines ...string) {
	if len(lines) == 0 {
		return
	}
	fmt.Println("  run it:")
	for _, l := range lines {
		fmt.Printf("    %s\n", l)
	}
}

func pathExists(p string) bool { _, err := os.Stat(p); return err == nil }

func removeAllPath(p string) error { return os.RemoveAll(p) }

func baseName(modulePath string) string { return filepath.Base(modulePath) }

// runCommandCapture runs a command, streams it to the terminal as usual, and
// also returns the combined output so the caller can diagnose the failure.
// Tools like xcodebuild bury the actionable line in a wall of build log.
func runCommandCapture(name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	return buf.String(), err
}

// removeToolPath deletes a go-installed binary and returns the path removed,
// or "" when it was not there. Returning the path rather than printing keeps
// formatting with the caller, which is what makes an aligned report possible.
func removeToolPath(tool string) string {
	for _, dir := range toolBinDirs() {
		p := filepath.Join(dir, exeName(tool))
		if _, err := os.Stat(p); err == nil {
			if os.Remove(p) == nil {
				return p
			}
		}
	}
	return ""
}

// runCommandQuiet runs a command and returns its combined output without
// echoing it. For bulk operations like a package removal, the tool's own
// output is hundreds of lines and buries the report it is part of — it is
// worth showing only when something failed.
func runCommandQuiet(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// moduleDir is where `go get` put a module, or "" if it is not a dependency.
//
// This is how anything reaches what ships inside a module — a kit's
// stylesheets, a skill, an example's assets — without a download or a second
// version to keep in step with go.mod's.
func moduleDir(mod string) string {
	out, err := exec.Command(goBin(), "list", "-m", "-f", "{{.Dir}}", mod).Output()
	if err != nil {
		return ""
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return ""
	}
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}

// copyGenerated writes src to dst unless dst already matches.
//
// Compared by content, not size or modification time. An upgrade that leaves a
// file the same length — a colour changed, a selector renamed — would be
// skipped by a size check, and the module cache stamps its own files when it
// extracts them, so times say nothing.
//
// 0644 rather than the source's mode: the module cache is read-only, and
// copying that through gives a project files it cannot overwrite next build.
func copyGenerated(src, dst string) error {
	want, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, want) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, want, 0o644)
}
