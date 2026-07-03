// Package native provides a uniform way to call platform capabilities
// (haptics, clipboard, share, secure storage, notifications, ...) from Go
// handler code — and to provide Go fallbacks so the same app code runs on
// web and desktop.
//
// Methods are namespaced strings like "haptics.impact" or "clipboard.write".
// On mobile, calls are routed to native plugins (Swift/Kotlin) registered
// with the Irgo shell. Elsewhere — or when no plugin claims the method — a
// Go handler registered with Register is used. If neither exists, Call
// returns ErrNotSupported.
//
//	result, err := native.Call(ctx, "haptics.impact", native.Params{"style": "medium"})
//
// The same methods are callable from the WebView via the JS API
// irgo.native(method, params), which resolves through native plugins on
// mobile and through the /_irgo/native HTTP endpoint (backed by this
// package) on web and desktop.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Params is a convenience type for building call parameters.
type Params map[string]any

// ErrNotSupported is returned when no native plugin and no Go handler
// implements the requested method on the current platform.
var ErrNotSupported = errors.New("native: method not supported on this platform")

// errNotSupportedMarker is the error string native plugins return to signal
// "no plugin claimed this method" so the call can fall back to Go handlers.
const errNotSupportedMarker = "IRGO_NOT_SUPPORTED"

// DefaultTimeout bounds native calls whose context has no deadline.
const DefaultTimeout = 30 * time.Second

// Handler implements a method in Go (used on web/desktop, or as a fallback
// when no native plugin claims the method).
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// Invoker dispatches a call to platform native code. It is implemented by
// the mobile bridge (backed by Swift/Kotlin plugin registries).
// Implementations must eventually call DeliverResult with the same callID.
type Invoker interface {
	Invoke(callID, method, paramsJSON string)
}

var (
	mu       sync.RWMutex
	handlers = make(map[string]Handler)
	invoker  Invoker

	pendingMu sync.Mutex
	pending   = make(map[string]chan callResult)
	callSeq   uint64
)

type callResult struct {
	ok      bool
	payload string
}

// Register adds a Go implementation for a method (e.g. "clipboard.write").
// On platforms with a native plugin for the method, the plugin wins; the Go
// handler is the fallback.
func Register(method string, h Handler) {
	mu.Lock()
	defer mu.Unlock()
	handlers[method] = h
}

// SetInvoker installs the platform dispatcher. Called by the mobile bridge
// during startup; app code should not need this.
func SetInvoker(inv Invoker) {
	mu.Lock()
	defer mu.Unlock()
	invoker = inv
}

// Call invokes a native method and returns its JSON result.
// Resolution order: platform invoker (native plugins) first, then Go
// handlers registered with Register, then ErrNotSupported.
func Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("native: encoding params: %w", err)
	}
	if params == nil {
		paramsJSON = []byte("{}")
	}

	mu.RLock()
	inv := invoker
	mu.RUnlock()

	if inv != nil {
		result, err := invokeNative(ctx, inv, method, string(paramsJSON))
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrNotSupported) {
			return nil, err
		}
		// Fall through to Go handlers.
	}

	return callGo(ctx, method, paramsJSON)
}

// callGo dispatches to a Go-registered handler only.
func callGo(ctx context.Context, method string, paramsJSON json.RawMessage) (json.RawMessage, error) {
	mu.RLock()
	h := handlers[method]
	mu.RUnlock()

	if h == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotSupported, method)
	}

	result, err := h(ctx, paramsJSON)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return json.RawMessage("null"), nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("native: encoding result: %w", err)
	}
	return data, nil
}

func invokeNative(ctx context.Context, inv Invoker, method, paramsJSON string) (json.RawMessage, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	ch := make(chan callResult, 1)
	pendingMu.Lock()
	callSeq++
	callID := "go-" + strconv.FormatUint(callSeq, 10)
	pending[callID] = ch
	pendingMu.Unlock()

	defer func() {
		pendingMu.Lock()
		delete(pending, callID)
		pendingMu.Unlock()
	}()

	inv.Invoke(callID, method, paramsJSON)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if !res.ok {
			if res.payload == errNotSupportedMarker {
				return nil, fmt.Errorf("%w: %s", ErrNotSupported, method)
			}
			return nil, errors.New("native: " + res.payload)
		}
		if res.payload == "" {
			return json.RawMessage("null"), nil
		}
		return json.RawMessage(res.payload), nil
	}
}

// DeliverResult completes a pending Call. Called by the mobile bridge when
// native code reports a result; app code should not need this.
// ok=false with payload "IRGO_NOT_SUPPORTED" triggers Go-handler fallback.
func DeliverResult(callID string, ok bool, payload string) {
	pendingMu.Lock()
	ch := pending[callID]
	delete(pending, callID)
	pendingMu.Unlock()

	if ch != nil {
		ch <- callResult{ok: ok, payload: payload}
	}
}

// HTTPHandler serves native calls over HTTP for web/desktop, where the
// WebView's irgo.native() cannot reach platform plugins. The router mounts
// it at POST /_irgo/native automatically.
//
// Request body: {"method": "...", "params": {...}}
// Response: {"ok": true, "result": ...} or {"ok": false, "error": "..."}
func HTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var call struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&call); err != nil || call.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	if len(call.Params) == 0 {
		call.Params = json.RawMessage("{}")
	}

	result, err := callGo(r.Context(), call.Method, call.Params)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotSupported) {
			status = http.StatusNotImplemented
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
