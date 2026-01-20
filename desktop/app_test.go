package desktop

import (
	"net/http"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Title != "Irgo App" {
		t.Errorf("expected default title 'Irgo App', got %q", config.Title)
	}
	if config.Width != 1024 {
		t.Errorf("expected default width 1024, got %d", config.Width)
	}
	if config.Height != 768 {
		t.Errorf("expected default height 768, got %d", config.Height)
	}
	if !config.Resizable {
		t.Error("expected default Resizable to be true")
	}
	if config.Debug {
		t.Error("expected default Debug to be false")
	}
	if config.Port != 0 {
		t.Errorf("expected default Port 0 (auto), got %d", config.Port)
	}
}

func TestNewApp(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	config := DefaultConfig()
	app := New(handler, config)

	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.handler == nil {
		t.Error("expected handler to be set")
	}
	if app.config.Title != config.Title {
		t.Error("expected config to be set")
	}
}

func TestAppFindPort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// Test auto port selection
	config := DefaultConfig()
	config.Port = 0
	app := New(handler, config)

	port, err := app.findPort()
	if err != nil {
		t.Fatalf("findPort failed: %v", err)
	}
	if port <= 0 {
		t.Errorf("expected positive port, got %d", port)
	}

	// Test specific port
	config.Port = 18080
	app2 := New(handler, config)
	port2, err := app2.findPort()
	if err != nil {
		t.Fatalf("findPort with specific port failed: %v", err)
	}
	if port2 != 18080 {
		t.Errorf("expected port 18080, got %d", port2)
	}
}

func TestAppURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	config := DefaultConfig()
	app := New(handler, config)
	app.port = 8080

	url := app.URL()
	expected := "http://127.0.0.1:8080"
	if url != expected {
		t.Errorf("expected URL %q, got %q", expected, url)
	}
}

func TestAppPort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	config := DefaultConfig()
	app := New(handler, config)
	app.port = 9999

	if app.Port() != 9999 {
		t.Errorf("expected port 9999, got %d", app.Port())
	}
}
