// Package adapter provides the virtual HTTP adapter that bridges
// core.Request/Response to standard net/http handlers.
package adapter

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/stukennedy/irgo/pkg/core"
)

// HTTPAdapter bridges core.Request/Response to net/http.Handler.
// This is the key component that enables "virtual HTTP" - executing
// HTTP handlers without any network I/O.
type HTTPAdapter struct {
	handler http.Handler
}

// NewHTTPAdapter creates an adapter for the given http.Handler.
func NewHTTPAdapter(handler http.Handler) *HTTPAdapter {
	return &HTTPAdapter{handler: handler}
}

// HandleRequest converts a core.Request, executes through the http.Handler,
// and returns a core.Response. This is the "virtual HTTP" implementation.
//
// No sockets are opened. The request is processed entirely in memory
// using httptest.ResponseRecorder.
func (a *HTTPAdapter) HandleRequest(req *core.Request) *core.Response {
	httpReq := buildHTTPRequest(req)

	// Create ResponseRecorder to capture output
	recorder := httptest.NewRecorder()

	// Execute handler directly - no network!
	a.handler.ServeHTTP(recorder, httpReq)

	// Convert back to core.Response
	result := recorder.Result()
	defer result.Body.Close()

	respBody, _ := io.ReadAll(result.Body)

	resp := &core.Response{
		Status: result.StatusCode,
		Body:   respBody,
	}
	resp.SetHTTPHeaders(result.Header)

	return resp
}

// buildHTTPRequest converts a core.Request into an *http.Request, preserving
// multi-valued headers. Shared by the buffered and streaming paths so request
// semantics cannot drift between them. Panics on malformed method/URL
// (httptest.NewRequest's contract) — callers recover where appropriate.
func buildHTTPRequest(req *core.Request) *http.Request {
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}

	httpReq := httptest.NewRequest(req.Method, req.URL, body)

	for k, values := range req.HTTPHeaders() {
		for _, v := range values {
			httpReq.Header.Add(k, v)
		}
	}
	return httpReq
}

// Handler returns the underlying http.Handler.
func (a *HTTPAdapter) Handler() http.Handler {
	return a.handler
}

// SetHandler updates the underlying http.Handler.
func (a *HTTPAdapter) SetHandler(handler http.Handler) {
	a.handler = handler
}

// HandlerFunc is a convenience type for creating adapters from functions.
type HandlerFunc func(http.ResponseWriter, *http.Request)

// NewHandlerFuncAdapter creates an adapter from a handler function.
func NewHandlerFuncAdapter(fn HandlerFunc) *HTTPAdapter {
	return NewHTTPAdapter(http.HandlerFunc(fn))
}
