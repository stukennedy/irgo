// Package core provides the fundamental types for the irgo framework.
// All types are designed to be gomobile-compatible (no maps, no slices of custom types).
package core

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Request represents an HTTP-like request from the mobile bridge.
// All fields use gomobile-compatible types.
type Request struct {
	Method  string // HTTP method: GET, POST, PUT, DELETE, PATCH
	URL     string // Full URL path with query string, e.g., "/tasks?filter=active"
	Headers string // JSON-encoded map[string]string for headers
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

// GetHeader returns the value of a header by key.
func (r *Request) GetHeader(key string) string {
	if r.Headers == "" || r.Headers == "{}" {
		return ""
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(r.Headers), &headers); err != nil {
		return ""
	}
	return headers[key]
}

// SetHeader sets a header value. Creates headers JSON if needed.
func (r *Request) SetHeader(key, value string) {
	var headers map[string]string
	if r.Headers == "" || r.Headers == "{}" {
		headers = make(map[string]string)
	} else {
		if err := json.Unmarshal([]byte(r.Headers), &headers); err != nil {
			headers = make(map[string]string)
		}
	}
	headers[key] = value
	data, _ := json.Marshal(headers)
	r.Headers = string(data)
}

// GetHeaders returns all headers as a map.
func (r *Request) GetHeaders() map[string]string {
	if r.Headers == "" || r.Headers == "{}" {
		return make(map[string]string)
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(r.Headers), &headers); err != nil {
		return make(map[string]string)
	}
	return headers
}

// SetHeaders sets all headers from a map.
func (r *Request) SetHeaders(headers map[string]string) {
	data, _ := json.Marshal(headers)
	r.Headers = string(data)
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

// IsHTMX returns true if this is an HTMX request (HX-Request header is "true").
func (r *Request) IsHTMX() bool {
	return r.GetHeader("HX-Request") == "true"
}

// ContentType returns the Content-Type header value.
func (r *Request) ContentType() string {
	return r.GetHeader("Content-Type")
}

// BodyString returns the body as a string.
func (r *Request) BodyString() string {
	return string(r.Body)
}
