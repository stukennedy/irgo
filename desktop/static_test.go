package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestStaticFS_DevMode(t *testing.T) {
	// Create a temp directory to simulate dev mode
	tmpDir, err := os.MkdirTemp("", "irgo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("dev content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create embedded FS (would be used in prod)
	embedded := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte("prod content")},
	}

	// StaticFS should prefer dev path when it exists
	fs := StaticFS(embedded, tmpDir)
	if fs == nil {
		t.Fatal("expected non-nil filesystem")
	}

	// Open the file through the filesystem
	f, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 100)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	if content != "dev content" {
		t.Errorf("expected dev content, got %q", content)
	}
}

func TestStaticFS_ProdMode(t *testing.T) {
	// Use a non-existent dev path
	nonExistentPath := "/this/path/does/not/exist"

	// Create embedded FS
	embedded := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte("prod content")},
	}

	// StaticFS should use embedded when dev path doesn't exist
	fs := StaticFS(embedded, nonExistentPath)
	if fs == nil {
		t.Fatal("expected non-nil filesystem")
	}

	// Open the file through the filesystem
	f, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 100)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	if content != "prod content" {
		t.Errorf("expected prod content, got %q", content)
	}
}

func TestFindStaticDir(t *testing.T) {
	// Save current dir
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create temp dir with static folder
	tmpDir, err := os.MkdirTemp("", "irgo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	staticDir := filepath.Join(tmpDir, "static")
	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}

	// Change to temp dir
	os.Chdir(tmpDir)

	result := FindStaticDir()
	if result != "static" {
		t.Errorf("expected 'static', got %q", result)
	}
}

func TestFindStaticDir_Fallback(t *testing.T) {
	// Save current dir
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create temp dir WITHOUT static folder
	tmpDir, err := os.MkdirTemp("", "irgo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp dir (no static folder)
	os.Chdir(tmpDir)

	result := FindStaticDir()
	// Should return "static" as fallback even if it doesn't exist
	if result != "static" {
		t.Errorf("expected fallback 'static', got %q", result)
	}
}
