package core

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Response represents the hypermedia response back to the WebView.
// Body contains HTML fragments or SSE events for Datastar to process.
//
// Headers is a JSON object. Each value may be either a string (the common
// case) or an array of strings for multi-valued headers such as Set-Cookie.
type Response struct {
	Status  int    // HTTP status code (200, 404, 500, etc.)
	Headers string // JSON-encoded headers: string or []string values
	Body    []byte // HTML fragment (or JSON for capability responses)
}

// NewResponse creates a new Response with the given status.
func NewResponse(status int) *Response {
	return &Response{
		Status:  status,
		Headers: "{}",
	}
}

// GetHeader returns a response header value.
// Header lookup is case-insensitive per HTTP spec.
// For multi-valued headers, the first value is returned.
func (r *Response) GetHeader(key string) string {
	return DecodeHeaders(r.Headers).Get(key)
}

// GetHeaderValues returns all values of a header (case-insensitive).
func (r *Response) GetHeaderValues(key string) []string {
	return DecodeHeaders(r.Headers).Values(key)
}

// SetHeader sets a response header, replacing any existing values.
func (r *Response) SetHeader(key, value string) {
	h := DecodeHeaders(r.Headers)
	h.Set(key, value)
	r.Headers = EncodeHeaders(h)
}

// AddHeader appends a response header value, preserving existing values.
func (r *Response) AddHeader(key, value string) {
	h := DecodeHeaders(r.Headers)
	h.Add(key, value)
	r.Headers = EncodeHeaders(h)
}

// GetHeaders returns all headers as a flat map (first value wins for
// multi-valued headers). Use HTTPHeaders for full multi-value access.
func (r *Response) GetHeaders() map[string]string {
	h := DecodeHeaders(r.Headers)
	headers := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers
}

// HTTPHeaders returns all headers as an http.Header.
func (r *Response) HTTPHeaders() http.Header {
	return DecodeHeaders(r.Headers)
}

// SetHeaders sets all headers from a map.
func (r *Response) SetHeaders(headers map[string]string) {
	h := make(http.Header, len(headers))
	for k, v := range headers {
		h.Set(k, v)
	}
	r.Headers = EncodeHeaders(h)
}

// SetHTTPHeaders sets all headers from an http.Header, preserving
// multi-valued headers such as Set-Cookie.
func (r *Response) SetHTTPHeaders(h http.Header) {
	r.Headers = EncodeHeaders(h)
}

// BodyString returns the body as a string.
func (r *Response) BodyString() string {
	return string(r.Body)
}

// SetBody sets the body from a string.
func (r *Response) SetBody(body string) {
	r.Body = []byte(body)
}

// HTMLResponse creates a response with HTML content.
func HTMLResponse(status int, html string) *Response {
	r := &Response{
		Status: status,
		Body:   []byte(html),
	}
	r.SetHeader("Content-Type", "text/html; charset=utf-8")
	return r
}

// JSONResponse creates a response with JSON content.
func JSONResponse(status int, data any) *Response {
	body, err := json.Marshal(data)
	if err != nil {
		return ErrorResponse(500, "JSON encoding error: "+err.Error())
	}
	r := &Response{
		Status: status,
		Body:   body,
	}
	r.SetHeader("Content-Type", "application/json")
	return r
}

// ErrorResponse creates an error response with the given status and message.
func ErrorResponse(status int, message string) *Response {
	html := fmt.Sprintf(`<div class="error" role="alert">%s</div>`, message)
	return HTMLResponse(status, html)
}

// RedirectResponse creates a redirect response.
// For Datastar requests, returns an SSE redirect event. Otherwise, sets Location header.
func RedirectResponse(url string, isDatastar bool) *Response {
	r := NewResponse(200)
	if isDatastar {
		// For Datastar, redirect is an SSE event
		r.SetHeader("Content-Type", "text/event-stream")
		r.Body = []byte(fmt.Sprintf("event: datastar-browser-redirect\ndata: url %s\n\n", url))
	} else {
		r.Status = 302
		r.SetHeader("Location", url)
	}
	return r
}

// SSEResponse creates an SSE response with the given event type and data.
func SSEResponse(eventType string, data string) *Response {
	r := NewResponse(200)
	r.SetHeader("Content-Type", "text/event-stream")
	r.Body = []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data))
	return r
}

// SSEPatchResponse creates an SSE response that patches HTML fragments.
func SSEPatchResponse(html string) *Response {
	return SSEResponse("datastar-patch-elements", "fragments "+html)
}

// SSESignalsResponse creates an SSE response that updates client signals.
func SSESignalsResponse(signalsJSON string) *Response {
	return SSEResponse("datastar-patch-signals", "signals "+signalsJSON)
}

// SSERemoveResponse creates an SSE response that removes elements.
func SSERemoveResponse(selector string) *Response {
	return SSEResponse("datastar-remove-elements", "selector "+selector)
}

// NoContentResponse creates a 204 No Content response.
func NoContentResponse() *Response {
	return NewResponse(204)
}

// NotFoundResponse creates a 404 Not Found response.
func NotFoundResponse(message string) *Response {
	if message == "" {
		message = "Not Found"
	}
	return ErrorResponse(404, message)
}

// BadRequestResponse creates a 400 Bad Request response.
func BadRequestResponse(message string) *Response {
	if message == "" {
		message = "Bad Request"
	}
	return ErrorResponse(400, message)
}

// InternalErrorResponse creates a 500 Internal Server Error response.
func InternalErrorResponse(message string) *Response {
	if message == "" {
		message = "Internal Server Error"
	}
	return ErrorResponse(500, message)
}
