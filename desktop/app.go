// Package desktop provides desktop application support using webview.
// Desktop mode uses a real HTTP server (like dev mode) with a native webview
// window pointing to localhost.
package desktop

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/stukennedy/irgo/pkg/transport"
	ws "github.com/stukennedy/irgo/pkg/websocket"
)

// Config holds desktop app configuration
type Config struct {
	Title     string
	Width     int
	Height    int
	Resizable bool
	Debug     bool   // Enable webview devtools
	Port      int    // 0 = auto-select available port
	Transport string // "loopback" (default) or "inprocess"
	Version   string // App version (shown in About menu on macOS)
	SetupMenu bool   // Setup native menu bar (macOS)
}

// DefaultConfig returns sensible defaults for a desktop app
func DefaultConfig() Config {
	return Config{
		Title:     "Irgo App",
		Width:     1024,
		Height:    768,
		Resizable: true,
		Debug:     false,
		Port:      0,
		Transport: "loopback",
		Version:   "1.0.0",
		SetupMenu: true,
	}
}

// App represents a desktop application with an embedded HTTP server
type App struct {
	config    Config
	handler   http.Handler
	wsHub     *ws.Hub
	transport transport.Transport
	wv        webview.WebView
	wg        sync.WaitGroup
}

// New creates a new desktop app with the given HTTP handler
func New(handler http.Handler, config Config) *App {
	return &App{
		config:  config,
		handler: handler,
		wsHub:   ws.NewHub(),
	}
}

// NewWithHub creates a new desktop app with a custom WebSocket hub
func NewWithHub(handler http.Handler, wsHub *ws.Hub, config Config) *App {
	return &App{
		config:  config,
		handler: handler,
		wsHub:   wsHub,
	}
}

// Run starts the desktop app (blocking until window is closed)
func (a *App) Run() error {
	// Setup native menu bar if enabled
	if a.config.SetupMenu {
		SetupMenu(a.config.Title, a.config.Version)
	}

	// Determine transport type from config or environment
	transportType := a.config.Transport
	if env := os.Getenv("IRGO_TRANSPORT"); env != "" {
		transportType = env
	}

	// Create the appropriate transport
	var t transport.Transport
	switch transportType {
	case "inprocess":
		t = transport.NewInProcessTransport(a.handler, a.wsHub,
			transport.WithPort(a.config.Port),
		)
	default:
		t = transport.NewLoopbackTransport(a.handler, a.wsHub,
			transport.WithPort(a.config.Port),
		)
	}
	a.transport = t

	// Start the transport
	if err := t.Start(); err != nil {
		return fmt.Errorf("starting transport: %w", err)
	}

	// Run webview (blocks until window closed)
	a.runWebview()

	// Cleanup
	return a.Shutdown()
}

// Port returns the port the server is running on (0 for inprocess transport)
func (a *App) Port() int {
	if a.transport == nil {
		return 0
	}
	cfg := a.transport.Config()
	if cfg != nil {
		return cfg.Port
	}
	return 0
}

// URL returns the local server URL (empty for inprocess transport)
func (a *App) URL() string {
	if a.transport == nil {
		return ""
	}
	cfg := a.transport.Config()
	if cfg != nil && cfg.Address != "" {
		return fmt.Sprintf("http://%s:%d", cfg.Address, cfg.Port)
	}
	return ""
}

// Secret returns the per-launch authentication secret
func (a *App) Secret() string {
	if a.transport == nil {
		return ""
	}
	cfg := a.transport.Config()
	if cfg != nil {
		return cfg.Secret
	}
	return ""
}

// Transport returns the underlying transport for advanced usage
func (a *App) Transport() transport.Transport {
	return a.transport
}

// Hub returns the WebSocket hub for registering handlers
func (a *App) Hub() *ws.Hub {
	return a.wsHub
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

	// Inject the secret into the webview before navigation
	// Using Init() ensures the script runs before any page scripts
	cfg := a.transport.Config()
	if cfg != nil && cfg.Secret != "" {
		js := "window.__IRGO_SECRET__ = '" + cfg.Secret + "';"
		a.wv.Init(js)
	}

	// Navigate to the server URL
	url := a.URL()
	if url != "" {
		a.wv.Navigate(url)
	}

	// Run blocks until window is closed
	a.wv.Run()
}

// Shutdown gracefully stops the app
func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if a.transport != nil {
		a.transport.Stop(ctx)
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

// RegisterChannelHandler registers a handler for WebSocket channels matching a pattern
func (a *App) RegisterChannelHandler(pattern string, handler transport.ChannelHandler) {
	if a.transport != nil {
		a.transport.RegisterChannelHandler(pattern, handler)
	}
}

// SetDefaultChannelHandler sets the default handler for WebSocket channels
func (a *App) SetDefaultChannelHandler(handler transport.ChannelHandler) {
	if a.transport != nil {
		a.transport.SetDefaultChannelHandler(handler)
	}
}
