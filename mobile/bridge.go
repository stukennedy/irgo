// Package mobile provides the gomobile-exported functions for iOS and Android.
// These functions are called by native code to handle HTTP requests and WebSocket
// messages without any network I/O.
package mobile

import (
	"net/http"
	"sync"

	"github.com/stukennedy/irgo/pkg/adapter"
	"github.com/stukennedy/irgo/pkg/core"
	"github.com/stukennedy/irgo/pkg/websocket"
)

var (
	globalBridge *Bridge
	bridgeMu     sync.RWMutex
)

// Bridge is the main interface between native code and Go.
type Bridge struct {
	adapter *adapter.HTTPAdapter
	wsHub   *websocket.Hub
	mu      sync.RWMutex
}

// NativeCallback is implemented by Swift/Kotlin to receive async callbacks.
type NativeCallback interface {
	// OnHTMLUpdate is called when new HTML should be swapped into the WebView.
	OnHTMLUpdate(targetSelector string, html string, swapStrategy string)

	// OnTrigger is called when HTMX triggers should fire.
	OnTrigger(eventName string, detail string)

	// OnError is called when an error occurs.
	OnError(code int, message string)
}

var nativeCallback NativeCallback

// Initialize creates and configures the global bridge.
// Must be called once at app startup from native code.
// If the bridge already exists (e.g., from SetHandler), this is a no-op.
func Initialize() {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()

	if globalBridge == nil {
		globalBridge = &Bridge{
			wsHub: websocket.NewHub(),
		}
	}
}

// SetHandler sets the HTTP handler for the bridge.
// This is called from Go app code after setting up routes.
func SetHandler(handler http.Handler) {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()

	if globalBridge == nil {
		globalBridge = &Bridge{
			wsHub: websocket.NewHub(),
		}
	}
	globalBridge.adapter = adapter.NewHTTPAdapter(handler)
}

// SetNativeCallback registers the native callback handler.
// Called from Swift/Kotlin during initialization.
func SetNativeCallback(cb NativeCallback) {
	nativeCallback = cb
}

// GetHub returns the WebSocket hub for handler registration.
func GetHub() *websocket.Hub {
	bridgeMu.RLock()
	defer bridgeMu.RUnlock()

	if globalBridge == nil {
		return nil
	}
	return globalBridge.wsHub
}

// HandleRequest processes an HTTP request and returns a response.
// This is the main entry point called by Swift/Kotlin for HTTP requests.
//
// Parameters are gomobile-compatible (no maps, no slices of custom types):
//   - method: HTTP method (GET, POST, etc.)
//   - url: Full URL path with query string
//   - headers: JSON-encoded map[string]string
//   - body: Request body bytes
func HandleRequest(method, url, headers string, body []byte) *core.Response {
	bridgeMu.RLock()
	b := globalBridge
	bridgeMu.RUnlock()

	if b == nil || b.adapter == nil {
		return core.ErrorResponse(500, "Bridge not initialized")
	}

	req := &core.Request{
		Method:  method,
		URL:     url,
		Headers: headers,
		Body:    body,
	}

	return b.adapter.HandleRequest(req)
}

// HandleRequestSimple is a simplified version for basic requests.
func HandleRequestSimple(method, url string) *core.Response {
	return HandleRequest(method, url, "{}", nil)
}

// RenderInitialPage renders the initial HTML page for the WebView.
// This is called once at app startup to get the initial content.
func RenderInitialPage() string {
	resp := HandleRequestSimple("GET", "/")
	if resp.Status >= 400 {
		return "<html><body><h1>Error loading app</h1></body></html>"
	}
	return resp.BodyString()
}

// IsReady returns true if the bridge is initialized and ready.
func IsReady() bool {
	bridgeMu.RLock()
	defer bridgeMu.RUnlock()
	return globalBridge != nil && globalBridge.adapter != nil
}

// Shutdown cleans up the bridge and closes all connections.
func Shutdown() {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()

	if globalBridge != nil {
		if globalBridge.wsHub != nil {
			globalBridge.wsHub.Close()
		}
		globalBridge = nil
	}
}
