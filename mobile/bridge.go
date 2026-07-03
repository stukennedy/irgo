// Package mobile provides the gomobile-exported functions for iOS and Android.
// These functions are called by native code to handle HTTP requests and WebSocket
// messages without any network I/O.
package mobile

import (
	"net/http"
	"strings"
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
	jar     *cookieJar
	mu      sync.RWMutex
}

// NativeCallback is implemented by Swift/Kotlin to receive async callbacks.
type NativeCallback interface {
	// OnHTMLUpdate is called when new HTML should be swapped into the WebView.
	OnHTMLUpdate(targetSelector string, html string, swapStrategy string)

	// OnTrigger is called when client-side events should fire.
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
	ensureBridgeLocked()
}

func ensureBridgeLocked() *Bridge {
	if globalBridge == nil {
		globalBridge = &Bridge{
			wsHub: websocket.NewHub(),
			jar:   newCookieJar(),
		}
	}
	return globalBridge
}

// SetHandler sets the HTTP handler for the bridge.
// This is called from Go app code after setting up routes.
func SetHandler(handler http.Handler) {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	ensureBridgeLocked().adapter = adapter.NewHTTPAdapter(handler)
}

// SetStateDir enables on-disk persistence for bridge state (currently the
// cookie jar, so login sessions survive app restarts). Native code should
// call this at startup with an app-private writable directory:
//   - iOS: Application Support or Documents directory
//   - Android: context.filesDir
func SetStateDir(dir string) {
	bridgeMu.Lock()
	b := ensureBridgeLocked()
	bridgeMu.Unlock()
	b.jar.setFile(cookieFilePath(dir))
}

// ClearCookies removes all cookies, including persisted ones.
// Useful for implementing logout.
func ClearCookies() {
	bridgeMu.RLock()
	b := globalBridge
	bridgeMu.RUnlock()
	if b != nil && b.jar != nil {
		b.jar.clear()
	}
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
	b.injectCookies(req)

	resp := b.adapter.HandleRequest(req)
	b.captureCookies(resp.Headers)
	return resp
}

// captureCookies stores Set-Cookie headers from a response into the jar.
// The substring guard avoids a full JSON decode of the header set on the
// hot path — the vast majority of responses (every static asset) set no
// cookies, and EncodeHeaders always emits the canonical "Set-Cookie" key.
func (b *Bridge) captureCookies(headersJSON string) {
	if b.jar == nil || !strings.Contains(headersJSON, "Set-Cookie") {
		return
	}
	if h := core.DecodeHeaders(headersJSON); len(h) > 0 {
		b.jar.setFromResponse(h)
	}
}

// injectCookies adds the jar's cookies to the request. WebViews don't manage
// cookies for custom schemes, so the Go side owns the cookie lifecycle.
func (b *Bridge) injectCookies(req *core.Request) {
	if b.jar == nil {
		return
	}
	if cookie := b.jar.cookieHeader(req.Path()); cookie != "" {
		req.AddHeader("Cookie", cookie)
	}
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
		if globalBridge.jar != nil {
			globalBridge.jar.save()
		}
		globalBridge = nil
	}
}
