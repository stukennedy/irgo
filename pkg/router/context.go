package router

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stukennedy/irgo/pkg/datastar"
)

// Context provides request data and response helpers for handlers.
type Context struct {
	Request  *http.Request
	Response http.ResponseWriter
	written  bool
}

// NewContext creates a new Context from the standard http types.
func NewContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{
		Request:  r,
		Response: w,
	}
}

// Context returns the request's context. It is cancelled when the client
// disconnects (including WebView cancellation on mobile), so pass it to
// native.Call, database queries, and long-lived work.
func (c *Context) Context() context.Context {
	return c.Request.Context()
}

// Param returns a URL path parameter extracted by chi router.
func (c *Context) Param(key string) string {
	return chi.URLParam(c.Request, key)
}

// Query returns a query string parameter.
func (c *Context) Query(key string) string {
	return c.Request.URL.Query().Get(key)
}

// QueryDefault returns a query parameter or default value if not present.
func (c *Context) QueryDefault(key, defaultValue string) string {
	v := c.Query(key)
	if v == "" {
		return defaultValue
	}
	return v
}

// FormValue returns a form field value (works for POST form data).
func (c *Context) FormValue(key string) string {
	return c.Request.FormValue(key)
}

// Header returns a request header value.
func (c *Context) Header(key string) string {
	return c.Request.Header.Get(key)
}

// SetHeader sets a response header.
func (c *Context) SetHeader(key, value string) {
	c.Response.Header().Set(key, value)
}

// --- Datastar Integration ---

// IsDatastar returns true if this is a Datastar request.
// Datastar sends an Accept header with text/event-stream for SSE requests.
func (c *Context) IsDatastar() bool {
	accept := c.Request.Header.Get("Accept")
	return accept == "text/event-stream"
}

// SSE creates a new SSE writer for streaming Datastar responses.
// Use this to send DOM patches, signal updates, and other SSE events.
func (c *Context) SSE() *datastar.SSE {
	return datastar.NewSSE(c.Response, c.Request)
}

// ReadSignals extracts Datastar signals from the request body.
// For GET requests, signals are read from URL query parameters.
// For other methods, signals are read from the JSON-encoded request body.
func (c *Context) ReadSignals(v any) error {
	return datastar.ReadSignals(c.Request, v)
}

// --- Standard HTTP Responses ---

// HTML writes an HTML response with 200 status.
func (c *Context) HTML(html string) {
	c.HTMLStatus(http.StatusOK, html)
}

// HTMLStatus writes an HTML response with custom status.
func (c *Context) HTMLStatus(status int, html string) {
	c.written = true
	c.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response.WriteHeader(status)
	c.Response.Write([]byte(html))
}

// JSON writes a JSON response with 200 status.
func (c *Context) JSON(data any) {
	c.JSONStatus(http.StatusOK, data)
}

// JSONStatus writes a JSON response with custom status.
func (c *Context) JSONStatus(status int, data any) {
	c.written = true
	c.Response.Header().Set("Content-Type", "application/json")
	c.Response.WriteHeader(status)
	json.NewEncoder(c.Response).Encode(data)
}

// Error writes an error response.
func (c *Context) Error(err error) {
	c.ErrorStatus(http.StatusInternalServerError, err.Error())
}

// ErrorStatus writes an error response with custom status.
func (c *Context) ErrorStatus(status int, message string) {
	c.written = true
	c.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response.WriteHeader(status)
	c.Response.Write([]byte(`<div class="error" role="alert">` + message + `</div>`))
}

// NotFound writes a 404 response.
func (c *Context) NotFound(message string) {
	if message == "" {
		message = "Not Found"
	}
	c.ErrorStatus(http.StatusNotFound, message)
}

// BadRequest writes a 400 response.
func (c *Context) BadRequest(message string) {
	if message == "" {
		message = "Bad Request"
	}
	c.ErrorStatus(http.StatusBadRequest, message)
}

// Redirect sends a standard HTTP redirect response.
func (c *Context) Redirect(url string) {
	c.written = true
	http.Redirect(c.Response, c.Request, url, http.StatusSeeOther)
}

// NoContent writes a 204 No Content response.
func (c *Context) NoContent() {
	c.written = true
	c.Response.WriteHeader(http.StatusNoContent)
}

// Written returns true if a response has been written.
func (c *Context) Written() bool {
	return c.written
}

// Bind decodes JSON body into the given struct.
func (c *Context) Bind(v any) error {
	return json.NewDecoder(c.Request.Body).Decode(v)
}
