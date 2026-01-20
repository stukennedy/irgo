package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

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

// getGoHTMXPath returns the path to the gohtmx source directory if developing locally
func getGoHTMXPath() string {
	// Check GOHTMX_PATH environment variable first
	if path := os.Getenv("GOHTMX_PATH"); path != "" {
		return path
	}

	// Helper to check if a directory contains gohtmx source
	isGoHTMXDir := func(dir string) bool {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			return strings.Contains(string(data), "module github.com/stukennedy/gohtmx")
		}
		return false
	}

	// Check if we're running from within or near the gohtmx source tree
	// by looking at current directory and parents
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 10; i++ {
			if isGoHTMXDir(dir) {
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
	// If gohtmx binary is at /path/to/gohtmx/cmd/gohtmx/gohtmx, source is at /path/to/gohtmx
	if execPath, err := os.Executable(); err == nil {
		execPath, _ = filepath.EvalSymlinks(execPath)
		execDir := filepath.Dir(execPath)

		// Check if we're in cmd/gohtmx directory
		if filepath.Base(filepath.Dir(execDir)) == "cmd" {
			possibleRoot := filepath.Dir(filepath.Dir(execDir))
			if isGoHTMXDir(possibleRoot) {
				return possibleRoot
			}
		}

		// Check parent directories of executable
		dir := execDir
		for i := 0; i < 5; i++ {
			if isGoHTMXDir(dir) {
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
		filepath.Join(home, "Dev", "gohtmx"),
		filepath.Join(home, "dev", "gohtmx"),
		filepath.Join(home, "Development", "gohtmx"),
		filepath.Join(home, "Projects", "gohtmx"),
		filepath.Join(home, "go", "src", "github.com", "stukennedy", "gohtmx"),
	}

	for _, path := range commonPaths {
		if isGoHTMXDir(path) {
			return path
		}
	}

	return ""
}

func newProject(name string) error {
	// Determine project directory and project name
	var projectDir string
	var projectName string

	if name == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		projectDir = cwd
		projectName = filepath.Base(cwd)
	} else if filepath.IsAbs(name) {
		// Absolute path provided
		projectDir = name
		projectName = filepath.Base(name)
	} else {
		projectDir = name
		projectName = name
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

	fmt.Printf("Creating new gohtmx project: %s\n", projectName)

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
		contentStr = strings.ReplaceAll(contentStr, "{{MODULE_PATH}}", "github.com/"+projectName)
		contentStr = strings.ReplaceAll(contentStr, "{{GO_VERSION}}", getGoVersion())

		// Add replace directive for local development if gohtmx isn't published
		gohtmxPath := getGoHTMXPath()
		if gohtmxPath != "" && strings.HasSuffix(relPath, "go.mod") {
			contentStr = strings.ReplaceAll(contentStr, "{{REPLACE_DIRECTIVE}}",
				fmt.Sprintf("\nreplace github.com/stukennedy/gohtmx => %s\n", gohtmxPath))
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

	// Make scripts executable
	scripts := []string{"dev.sh"}
	for _, script := range scripts {
		path := filepath.Join(projectDir, script)
		if err := os.Chmod(path, 0755); err != nil {
			// Ignore if file doesn't exist
			continue
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
	fmt.Println("  go mod tidy")
	fmt.Println("  bun install        # or: npm install")
	fmt.Println("  gohtmx dev         # start development server")
	fmt.Println()

	return nil
}
