// Package desktop provides desktop application support using webview.
// Desktop mode uses a real HTTP server (like dev mode) with a native webview
// window pointing to localhost.
package desktop

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	webview "github.com/webview/webview_go"
)

// Config holds desktop app configuration
type Config struct {
	Title     string
	Width     int
	Height    int
	Resizable bool
	Debug     bool // Enable webview devtools
	Port      int  // 0 = auto-select available port
}

// DefaultConfig returns sensible defaults for a desktop app
func DefaultConfig() Config {
	return Config{
		Title:     "GoHTMX App",
		Width:     1024,
		Height:    768,
		Resizable: true,
		Debug:     false,
		Port:      0,
	}
}

// App represents a desktop application with an embedded HTTP server
type App struct {
	config  Config
	handler http.Handler
	server  *http.Server
	wv      webview.WebView
	port    int
	wg      sync.WaitGroup
}

// New creates a new desktop app with the given HTTP handler
func New(handler http.Handler, config Config) *App {
	return &App{
		config:  config,
		handler: handler,
	}
}

// Run starts the desktop app (blocking until window is closed)
func (a *App) Run() error {
	// 1. Find available port
	port, err := a.findPort()
	if err != nil {
		return fmt.Errorf("finding port: %w", err)
	}
	a.port = port

	// 2. Start HTTP server in background
	if err := a.startServer(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	// 3. Create and run webview (blocks until window closed)
	a.runWebview()

	// 4. Cleanup
	return a.Shutdown()
}

// Port returns the port the server is running on
func (a *App) Port() int {
	return a.port
}

// URL returns the local server URL
func (a *App) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", a.port)
}

func (a *App) findPort() (int, error) {
	if a.config.Port != 0 {
		return a.config.Port, nil
	}
	// Find random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port, nil
}

func (a *App) startServer() error {
	a.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", a.port),
		Handler: a.handler,
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.server.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	return nil
}

func (a *App) runWebview() {
	a.wv = webview.New(a.config.Debug)
	defer a.wv.Destroy()

	a.wv.SetTitle(a.config.Title)

	if a.config.Resizable {
		a.wv.SetSize(a.config.Width, a.config.Height, webview.HintNone)
	} else {
		a.wv.SetSize(a.config.Width, a.config.Height, webview.HintFixed)
	}

	a.wv.Navigate(a.URL())

	// Run blocks until window is closed
	a.wv.Run()
}

// Shutdown gracefully stops the app
func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if a.server != nil {
		a.server.Shutdown(ctx)
	}

	a.wg.Wait()
	return nil
}

// Bind binds a Go function to a JavaScript name in the webview
func (a *App) Bind(name string, fn interface{}) error {
	if a.wv == nil {
		return fmt.Errorf("webview not initialized")
	}
	a.wv.Bind(name, fn)
	return nil
}

// Eval evaluates JavaScript in the webview
func (a *App) Eval(js string) {
	if a.wv != nil {
		a.wv.Eval(js)
	}
}
