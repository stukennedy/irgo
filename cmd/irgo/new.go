package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

// HTMX files to download during project creation
var htmxFiles = map[string]string{
	"static/js/htmx.min.js": "https://four.htmx.org/js/htmx.min.js",
	"static/js/hx-ws.js":    "https://four.htmx.org/js/ext/hx-ws.js",
}

// downloadHTMX downloads HTMX files to the project's static/js directory
func downloadHTMX(projectDir string) error {
	for destPath, url := range htmxFiles {
		fullPath := filepath.Join(projectDir, destPath)

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", destPath, err)
		}

		// Download the file
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("downloading %s: %w", url, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("downloading %s: status %d", url, resp.StatusCode)
		}

		// Read the content
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading %s: %w", url, err)
		}

		// Write to file
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		fmt.Printf("  downloaded: %s\n", destPath)
	}

	return nil
}

// getGoVersion returns the current Go version (e.g., "1.24.12")
func getGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "1.23"
	}
	// Parse "go version go1.24.12 darwin/arm64"
	re := regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`)
	match := re.FindStringSubmatch(string(out))
	if len(match) > 1 {
		return match[1]
	}
	return "1.23"
}

// getIrgoPath returns the path to the irgo source directory if developing locally
func getIrgoPath() string {
	// Check IRGO_PATH environment variable first
	if path := os.Getenv("IRGO_PATH"); path != "" {
		return path
	}

	// Helper to check if a directory contains irgo source
	isIrgoDir := func(dir string) bool {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			return strings.Contains(string(data), "module github.com/stukennedy/irgo")
		}
		return false
	}

	// Check if we're running from within or near the irgo source tree
	// by looking at current directory and parents
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 10; i++ {
			if isIrgoDir(dir) {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Check relative to executable location
	// If irgo binary is at /path/to/irgo/cmd/irgo/irgo, source is at /path/to/irgo
	if execPath, err := os.Executable(); err == nil {
		execPath, _ = filepath.EvalSymlinks(execPath)
		execDir := filepath.Dir(execPath)

		// Check if we're in cmd/irgo directory
		if filepath.Base(filepath.Dir(execDir)) == "cmd" {
			possibleRoot := filepath.Dir(filepath.Dir(execDir))
			if isIrgoDir(possibleRoot) {
				return possibleRoot
			}
		}

		// Check parent directories of executable
		dir := execDir
		for i := 0; i < 5; i++ {
			if isIrgoDir(dir) {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Check common development locations
	home, _ := os.UserHomeDir()
	commonPaths := []string{
		filepath.Join(home, "Dev", "@irgo", "core"),
		filepath.Join(home, "Dev", "irgo"),
		filepath.Join(home, "dev", "irgo"),
		filepath.Join(home, "Development", "irgo"),
		filepath.Join(home, "Projects", "irgo"),
		filepath.Join(home, "go", "src", "github.com", "stukennedy", "irgo"),
	}

	for _, path := range commonPaths {
		if isIrgoDir(path) {
			return path
		}
	}

	return ""
}

// isRemoteModulePath checks if a path looks like a remote Go module path
func isRemoteModulePath(path string) bool {
	remotePrefixes := []string{
		"github.com/",
		"gitlab.com/",
		"bitbucket.org/",
		"gopkg.in/",
	}
	for _, prefix := range remotePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	// Any path with dots before slashes is likely remote
	if strings.Contains(path, ".") && strings.Contains(path, "/") {
		dotIdx := strings.Index(path, ".")
		slashIdx := strings.Index(path, "/")
		if dotIdx < slashIdx {
			return true
		}
	}
	return false
}

func newProject(name string) error {
	// Determine project directory, project name, and module path
	var projectDir string
	var projectName string
	var modulePath string

	if name == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		projectDir = cwd
		projectName = filepath.Base(cwd)
		modulePath = projectName
	} else if filepath.IsAbs(name) {
		// Absolute path provided
		projectDir = name
		projectName = filepath.Base(name)
		modulePath = projectName
	} else if isRemoteModulePath(name) {
		// Remote module path like "github.com/user/project"
		// Use the last part for directory, full path for module
		projectDir = filepath.Base(name)
		projectName = filepath.Base(name)
		modulePath = name
	} else {
		projectDir = name
		projectName = name
		modulePath = name
	}

	// Check if directory exists and is not empty
	if name != "." {
		if _, err := os.Stat(projectDir); err == nil {
			entries, _ := os.ReadDir(projectDir)
			if len(entries) > 0 {
				return fmt.Errorf("directory %s already exists and is not empty", projectDir)
			}
		}
	}

	fmt.Printf("Creating new irgo project: %s\n", projectName)

	// Create project structure
	dirs := []string{
		"handlers",
		"templates",
		"static/css",
		"static/js",
	}

	for _, dir := range dirs {
		path := filepath.Join(projectDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", path, err)
		}
	}

	// Copy template files
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root templates directory
		if path == "templates" {
			return nil
		}

		// Get relative path from templates/
		relPath := strings.TrimPrefix(path, "templates/")
		destPath := filepath.Join(projectDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Read template file
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		// Handle .tmpl extension (remove it) - do this before checking file type
		if strings.HasSuffix(destPath, ".tmpl") {
			destPath = strings.TrimSuffix(destPath, ".tmpl")
			relPath = strings.TrimSuffix(relPath, ".tmpl")
		}

		// Replace placeholders
		contentStr := string(content)
		contentStr = strings.ReplaceAll(contentStr, "{{PROJECT_NAME}}", projectName)
		contentStr = strings.ReplaceAll(contentStr, "{{MODULE_PATH}}", modulePath)
		contentStr = strings.ReplaceAll(contentStr, "{{GO_VERSION}}", getGoVersion())

		// Add replace directive for local development if irgo isn't published
		irgoPath := getIrgoPath()
		if irgoPath != "" && strings.HasSuffix(relPath, "go.mod") {
			contentStr = strings.ReplaceAll(contentStr, "{{REPLACE_DIRECTIVE}}",
				fmt.Sprintf("\nreplace github.com/stukennedy/irgo => %s\n", irgoPath))
		} else {
			contentStr = strings.ReplaceAll(contentStr, "{{REPLACE_DIRECTIVE}}", "")
		}

		// Write file
		if err := os.WriteFile(destPath, []byte(contentStr), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		fmt.Printf("  created: %s\n", relPath)
		return nil
	})

	if err != nil {
		return fmt.Errorf("copying templates: %w", err)
	}

	// Download HTMX files
	fmt.Println("Downloading HTMX...")
	if err := downloadHTMX(projectDir); err != nil {
		return fmt.Errorf("downloading HTMX: %w", err)
	}

	// Make scripts executable
	scripts := []string{"dev.sh"}
	for _, script := range scripts {
		path := filepath.Join(projectDir, script)
		if err := os.Chmod(path, 0755); err != nil {
			// Ignore if file doesn't exist
			continue
		}
	}

	// Run go mod tidy to download dependencies
	// Skip if it's a remote module path that doesn't exist yet
	if isRemoteModulePath(modulePath) {
		fmt.Println("Skipping go mod tidy (remote module path - run manually after pushing to remote)")
	} else {
		fmt.Println("Running go mod tidy...")
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = projectDir
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			fmt.Printf("Warning: go mod tidy failed: %v\n", err)
		}
	}

	// Generate templ files if templ is available
	if _, err := exec.LookPath("templ"); err == nil {
		fmt.Println("Generating templ files...")
		cmd := exec.Command("templ", "generate")
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: templ generate failed: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("Project created successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", projectDir)
	fmt.Println("  bun install        # or: npm install")
	fmt.Println("  irgo dev           # start development server")
	fmt.Println()

	return nil
}
