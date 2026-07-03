package mobile

import (
	"context"
	"sync"

	"github.com/stukennedy/irgo/pkg/core"
)

// StreamCallback is implemented by Swift/Kotlin to receive a response
// progressively. This is how SSE works on mobile: each flush on the Go side
// becomes one OnChunk on the native side, delivered while the handler is
// still running.
//
// Callbacks arrive sequentially from a background goroutine: OnResponse
// once, OnChunk zero or more times, OnComplete once (always last).
type StreamCallback interface {
	// OnResponse delivers the status code and JSON-encoded headers.
	OnResponse(status int, headersJSON string)

	// OnChunk delivers a piece of the response body.
	OnChunk(chunk []byte)

	// OnComplete signals the end of the response. errorMessage is "" on
	// success.
	OnComplete(errorMessage string)
}

// StreamHandle lets native code cancel an in-flight streaming request, e.g.
// when the WebView navigates away or the scheme task is stopped. Cancelling
// cancels the handler's request context, so datastar handlers observing
// SSE.IsClosed()/Context() terminate just like on a real disconnect.
type StreamHandle struct {
	cancel context.CancelFunc
	once   sync.Once
}

// Cancel aborts the request. Safe to call multiple times and after
// completion.
func (h *StreamHandle) Cancel() {
	h.once.Do(h.cancel)
}

// HandleRequestStream processes an HTTP request, streaming the response
// through the callback. It returns immediately; callbacks are delivered from
// a background goroutine.
//
// Use this for Datastar/SSE requests (Accept: text/event-stream) and any
// long-lived response. Buffered HandleRequest remains available for simple
// resource loads.
func HandleRequestStream(method, url, headers string, body []byte, callback StreamCallback) *StreamHandle {
	bridgeMu.RLock()
	b := globalBridge
	bridgeMu.RUnlock()

	ctx, cancel := context.WithCancel(context.Background())
	handle := &StreamHandle{cancel: cancel}

	if b == nil || b.adapter == nil {
		go func() {
			defer cancel()
			// Same error shape as the buffered path (core.ErrorResponse).
			resp := core.ErrorResponse(500, "Bridge not initialized")
			callback.OnResponse(resp.Status, resp.Headers)
			callback.OnChunk(resp.Body)
			callback.OnComplete("Bridge not initialized")
		}()
		return handle
	}

	req := &core.Request{
		Method:  method,
		URL:     url,
		Headers: headers,
		Body:    body,
	}
	b.injectCookies(req)

	go func() {
		defer cancel()
		b.adapter.HandleRequestStream(ctx, req, &cookieAwareStream{callback: callback, bridge: b})
	}()

	return handle
}

// cookieAwareStream forwards stream events to the native callback while
// capturing Set-Cookie headers into the bridge's jar.
type cookieAwareStream struct {
	callback StreamCallback
	bridge   *Bridge
}

func (s *cookieAwareStream) OnResponse(status int, headersJSON string) {
	s.bridge.captureCookies(headersJSON)
	s.callback.OnResponse(status, headersJSON)
}

func (s *cookieAwareStream) OnChunk(chunk []byte) {
	s.callback.OnChunk(chunk)
}

func (s *cookieAwareStream) OnComplete(errorMessage string) {
	s.callback.OnComplete(errorMessage)
}
