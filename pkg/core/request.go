// Package core provides the fundamental types for the irgo framework.
// All types are designed to be gomobile-compatible (no maps, no slices of custom types).
package core

import (
	"net/http"
	"net/url"
	"strings"
)

// Request represents an HTTP-like request from the mobile bridge.
// All fields use gomobile-compatible types.
//
// Headers is a JSON object. Each value may be either a string (the common
// case) or an array of strings for multi-valued headers such as Cookie.
type Request struct {
	Method  string // HTTP method: GET, POST, PUT, DELETE, PATCH
	URL     string // Full URL path with query string, e.g., "/tasks?filter=active"
	Headers string // JSON-encoded headers: string or []string values
	Body    []byte // Request body (form data, JSON, etc.)
}

// NewRequest creates a new Request with the given method and URL.
func NewRequest(method, url string) *Request {
	return &Request{
		Method:  method,
		URL:     url,
		Headers: "{}",
	}
}

// GetHeader returns the value of a header by key (case-insensitive).
// For multi-valued headers, the first value is returned.
func (r *Request) GetHeader(key string) string {
	return DecodeHeaders(r.Headers).Get(key)
}

// GetHeaderValues returns all values of a header by key (case-insensitive).
func (r *Request) GetHeaderValues(key string) []string {
	return DecodeHeaders(r.Headers).Values(key)
}

// SetHeader sets a header value, replacing any existing values.
func (r *Request) SetHeader(key, value string) {
	h := DecodeHeaders(r.Headers)
	h.Set(key, value)
	r.Headers = EncodeHeaders(h)
}

// AddHeader appends a header value, preserving existing values.
func (r *Request) AddHeader(key, value string) {
	h := DecodeHeaders(r.Headers)
	h.Add(key, value)
	r.Headers = EncodeHeaders(h)
}

// GetHeaders returns all headers as a flat map (first value wins for
// multi-valued headers). Use HTTPHeaders for full multi-value access.
func (r *Request) GetHeaders() map[string]string {
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
func (r *Request) HTTPHeaders() http.Header {
	return DecodeHeaders(r.Headers)
}

// SetHeaders sets all headers from a map.
func (r *Request) SetHeaders(headers map[string]string) {
	h := make(http.Header, len(headers))
	for k, v := range headers {
		h.Set(k, v)
	}
	r.Headers = EncodeHeaders(h)
}

// SetHTTPHeaders sets all headers from an http.Header.
func (r *Request) SetHTTPHeaders(h http.Header) {
	r.Headers = EncodeHeaders(h)
}

// Path returns the URL path without query string.
func (r *Request) Path() string {
	if idx := strings.Index(r.URL, "?"); idx != -1 {
		return r.URL[:idx]
	}
	return r.URL
}

// Query returns the raw query string.
func (r *Request) Query() string {
	if idx := strings.Index(r.URL, "?"); idx != -1 {
		return r.URL[idx+1:]
	}
	return ""
}

// QueryValue returns a query parameter value.
func (r *Request) QueryValue(key string) string {
	query := r.Query()
	if query == "" {
		return ""
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return ""
	}
	return values.Get(key)
}

// IsDatastar returns true if this is a Datastar SSE request (Accept header is "text/event-stream").
func (r *Request) IsDatastar() bool {
	return r.GetHeader("Accept") == "text/event-stream"
}

// ContentType returns the Content-Type header value.
func (r *Request) ContentType() string {
	return r.GetHeader("Content-Type")
}

// BodyString returns the body as a string.
func (r *Request) BodyString() string {
	return string(r.Body)
}
