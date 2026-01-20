// Package testing provides utilities for testing irgo applications.
//
// Example usage:
//
//	func TestHomeHandler(t *testing.T) {
//	    r := app.NewRouter()
//	    client := testing.NewClient(r)
//
//	    // Simple GET request
//	    resp := client.Get("/")
//	    resp.AssertStatus(t, 200)
//	    resp.AssertContains(t, "Welcome")
//
//	    // POST with form data
//	    resp = client.PostForm("/todos", map[string]string{"title": "New Todo"})
//	    resp.AssertStatus(t, 200)
//
//	    // HTMX request
//	    resp = client.HTMX().Get("/fragment")
//	    resp.AssertStatus(t, 200)
//	}
package testing

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Client provides test utilities for irgo applications.
type Client struct {
	handler http.Handler
	headers map[string]string
}

// NewClient creates a new test client for the given handler.
func NewClient(handler http.Handler) *Client {
	return &Client{
		handler: handler,
		headers: make(map[string]string),
	}
}

// WithHeader returns a new client with the specified header set.
func (c *Client) WithHeader(key, value string) *Client {
	newClient := &Client{
		handler: c.handler,
		headers: make(map[string]string),
	}
	for k, v := range c.headers {
		newClient.headers[k] = v
	}
	newClient.headers[key] = value
	return newClient
}

// HTMX returns a client configured for HTMX requests.
func (c *Client) HTMX() *Client {
	return c.WithHeader("HX-Request", "true")
}

// HTMXWithTarget returns a client configured for HTMX requests with a specific target.
func (c *Client) HTMXWithTarget(target string) *Client {
	return c.HTMX().WithHeader("HX-Target", target)
}

// Get performs a GET request.
func (c *Client) Get(path string) *Response {
	return c.request("GET", path, nil)
}

// Post performs a POST request with the given body.
func (c *Client) Post(path string, body io.Reader) *Response {
	return c.request("POST", path, body)
}

// PostForm performs a POST request with form data.
func (c *Client) PostForm(path string, data map[string]string) *Response {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	return c.WithHeader("Content-Type", "application/x-www-form-urlencoded").
		request("POST", path, strings.NewReader(form.Encode()))
}

// PostJSON performs a POST request with JSON body.
func (c *Client) PostJSON(path string, jsonBody string) *Response {
	return c.WithHeader("Content-Type", "application/json").
		request("POST", path, strings.NewReader(jsonBody))
}

// Put performs a PUT request with the given body.
func (c *Client) Put(path string, body io.Reader) *Response {
	return c.request("PUT", path, body)
}

// PutForm performs a PUT request with form data.
func (c *Client) PutForm(path string, data map[string]string) *Response {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	return c.WithHeader("Content-Type", "application/x-www-form-urlencoded").
		request("PUT", path, strings.NewReader(form.Encode()))
}

// PutJSON performs a PUT request with JSON body.
func (c *Client) PutJSON(path string, jsonBody string) *Response {
	return c.WithHeader("Content-Type", "application/json").
		request("PUT", path, strings.NewReader(jsonBody))
}

// Patch performs a PATCH request with the given body.
func (c *Client) Patch(path string, body io.Reader) *Response {
	return c.request("PATCH", path, body)
}

// PatchJSON performs a PATCH request with JSON body.
func (c *Client) PatchJSON(path string, jsonBody string) *Response {
	return c.WithHeader("Content-Type", "application/json").
		request("PATCH", path, strings.NewReader(jsonBody))
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string) *Response {
	return c.request("DELETE", path, nil)
}

func (c *Client) request(method, path string, body io.Reader) *Response {
	req := httptest.NewRequest(method, path, body)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, req)

	return &Response{
		StatusCode: w.Code,
		Headers:    w.Header(),
		Body:       w.Body.Bytes(),
	}
}

// Response represents the result of a test request.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// BodyString returns the response body as a string.
func (r *Response) BodyString() string {
	return string(r.Body)
}

// Header returns the value of a response header.
func (r *Response) Header(key string) string {
	return r.Headers.Get(key)
}

// AssertStatus asserts the response status code.
func (r *Response) AssertStatus(t *testing.T, expected int) {
	t.Helper()
	if r.StatusCode != expected {
		t.Errorf("expected status %d, got %d\nBody: %s", expected, r.StatusCode, r.BodyString())
	}
}

// AssertOK asserts the response status is 200.
func (r *Response) AssertOK(t *testing.T) {
	t.Helper()
	r.AssertStatus(t, http.StatusOK)
}

// AssertCreated asserts the response status is 201.
func (r *Response) AssertCreated(t *testing.T) {
	t.Helper()
	r.AssertStatus(t, http.StatusCreated)
}

// AssertNotFound asserts the response status is 404.
func (r *Response) AssertNotFound(t *testing.T) {
	t.Helper()
	r.AssertStatus(t, http.StatusNotFound)
}

// AssertBadRequest asserts the response status is 400.
func (r *Response) AssertBadRequest(t *testing.T) {
	t.Helper()
	r.AssertStatus(t, http.StatusBadRequest)
}

// AssertNoContent asserts the response status is 204.
func (r *Response) AssertNoContent(t *testing.T) {
	t.Helper()
	r.AssertStatus(t, http.StatusNoContent)
}

// AssertRedirect asserts the response is a redirect (3xx status).
func (r *Response) AssertRedirect(t *testing.T) {
	t.Helper()
	if r.StatusCode < 300 || r.StatusCode >= 400 {
		t.Errorf("expected redirect (3xx), got %d", r.StatusCode)
	}
}

// AssertContains asserts the response body contains the given string.
func (r *Response) AssertContains(t *testing.T, expected string) {
	t.Helper()
	if !strings.Contains(r.BodyString(), expected) {
		t.Errorf("expected body to contain %q\nBody: %s", expected, r.BodyString())
	}
}

// AssertNotContains asserts the response body does not contain the given string.
func (r *Response) AssertNotContains(t *testing.T, unexpected string) {
	t.Helper()
	if strings.Contains(r.BodyString(), unexpected) {
		t.Errorf("expected body to not contain %q\nBody: %s", unexpected, r.BodyString())
	}
}

// AssertBodyEquals asserts the response body exactly matches the given string.
func (r *Response) AssertBodyEquals(t *testing.T, expected string) {
	t.Helper()
	if r.BodyString() != expected {
		t.Errorf("expected body %q, got %q", expected, r.BodyString())
	}
}

// AssertHeader asserts a response header has the expected value.
func (r *Response) AssertHeader(t *testing.T, key, expected string) {
	t.Helper()
	actual := r.Headers.Get(key)
	if actual != expected {
		t.Errorf("expected header %s=%q, got %q", key, expected, actual)
	}
}

// AssertHeaderExists asserts a response header exists.
func (r *Response) AssertHeaderExists(t *testing.T, key string) {
	t.Helper()
	if r.Headers.Get(key) == "" {
		t.Errorf("expected header %s to exist", key)
	}
}

// AssertHTMXTrigger asserts the HX-Trigger header has the expected value.
func (r *Response) AssertHTMXTrigger(t *testing.T, expected string) {
	t.Helper()
	r.AssertHeader(t, "HX-Trigger", expected)
}

// AssertHTMXRedirect asserts the HX-Redirect header has the expected value.
func (r *Response) AssertHTMXRedirect(t *testing.T, expected string) {
	t.Helper()
	r.AssertHeader(t, "HX-Redirect", expected)
}

// AssertHTMXRefresh asserts the HX-Refresh header is set.
func (r *Response) AssertHTMXRefresh(t *testing.T) {
	t.Helper()
	r.AssertHeader(t, "HX-Refresh", "true")
}

// AssertContentType asserts the Content-Type header.
func (r *Response) AssertContentType(t *testing.T, expected string) {
	t.Helper()
	ct := r.Headers.Get("Content-Type")
	if !strings.HasPrefix(ct, expected) {
		t.Errorf("expected Content-Type %q, got %q", expected, ct)
	}
}

// AssertHTML asserts the Content-Type is text/html.
func (r *Response) AssertHTML(t *testing.T) {
	t.Helper()
	r.AssertContentType(t, "text/html")
}

// AssertJSON asserts the Content-Type is application/json.
func (r *Response) AssertJSON(t *testing.T) {
	t.Helper()
	r.AssertContentType(t, "application/json")
}

// ContainsAll checks if the body contains all the given strings.
func (r *Response) ContainsAll(strs ...string) bool {
	body := r.BodyString()
	for _, s := range strs {
		if !strings.Contains(body, s) {
			return false
		}
	}
	return true
}

// AssertContainsAll asserts the body contains all the given strings.
func (r *Response) AssertContainsAll(t *testing.T, strs ...string) {
	t.Helper()
	body := r.BodyString()
	for _, s := range strs {
		if !strings.Contains(body, s) {
			t.Errorf("expected body to contain %q\nBody: %s", s, body)
		}
	}
}

// HTMLContains provides HTML-aware content checking.
type HTMLAssertions struct {
	t    *testing.T
	body string
}

// HTML returns HTML assertion helpers for the response.
func (r *Response) HTML(t *testing.T) *HTMLAssertions {
	t.Helper()
	return &HTMLAssertions{t: t, body: r.BodyString()}
}

// ContainsElement asserts the HTML contains an element with the given tag and attributes.
// This is a simple string-based check, not a full HTML parser.
func (h *HTMLAssertions) ContainsElement(tag string, attrs ...string) {
	h.t.Helper()
	if !strings.Contains(h.body, "<"+tag) {
		h.t.Errorf("expected HTML to contain <%s> element\nBody: %s", tag, h.body)
		return
	}
	for _, attr := range attrs {
		if !strings.Contains(h.body, attr) {
			h.t.Errorf("expected HTML to contain attribute %q\nBody: %s", attr, h.body)
		}
	}
}

// ContainsID asserts the HTML contains an element with the given ID.
func (h *HTMLAssertions) ContainsID(id string) {
	h.t.Helper()
	if !strings.Contains(h.body, `id="`+id+`"`) && !strings.Contains(h.body, `id='`+id+`'`) {
		h.t.Errorf("expected HTML to contain element with id=%q\nBody: %s", id, h.body)
	}
}

// ContainsClass asserts the HTML contains an element with the given class.
func (h *HTMLAssertions) ContainsClass(class string) {
	h.t.Helper()
	if !strings.Contains(h.body, class) {
		h.t.Errorf("expected HTML to contain class %q\nBody: %s", class, h.body)
	}
}

// MockRenderer is a test renderer that captures rendered templates.
type MockRenderer struct {
	Rendered []string
}

// Render captures the template call and returns a placeholder.
func (m *MockRenderer) Render(component interface{}) (string, error) {
	m.Rendered = append(m.Rendered, "<mock-rendered/>")
	return "<mock-rendered/>", nil
}

// AssertRenderedCount asserts the number of times Render was called.
func (m *MockRenderer) AssertRenderedCount(t *testing.T, expected int) {
	t.Helper()
	if len(m.Rendered) != expected {
		t.Errorf("expected %d renders, got %d", expected, len(m.Rendered))
	}
}

// Reset clears the render history.
func (m *MockRenderer) Reset() {
	m.Rendered = nil
}

// NewTestServer creates an httptest.Server for integration tests.
func NewTestServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(handler)
}

// RequestBuilder provides a fluent API for building test requests.
type RequestBuilder struct {
	method  string
	path    string
	headers map[string]string
	body    io.Reader
}

// NewRequest creates a new request builder.
func NewRequest(method, path string) *RequestBuilder {
	return &RequestBuilder{
		method:  method,
		path:    path,
		headers: make(map[string]string),
	}
}

// WithHeader adds a header to the request.
func (rb *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	rb.headers[key] = value
	return rb
}

// WithBody sets the request body.
func (rb *RequestBuilder) WithBody(body io.Reader) *RequestBuilder {
	rb.body = body
	return rb
}

// WithFormBody sets form data as the request body.
func (rb *RequestBuilder) WithFormBody(data map[string]string) *RequestBuilder {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	rb.body = strings.NewReader(form.Encode())
	rb.headers["Content-Type"] = "application/x-www-form-urlencoded"
	return rb
}

// WithJSONBody sets JSON as the request body.
func (rb *RequestBuilder) WithJSONBody(json string) *RequestBuilder {
	rb.body = strings.NewReader(json)
	rb.headers["Content-Type"] = "application/json"
	return rb
}

// AsHTMX marks this as an HTMX request.
func (rb *RequestBuilder) AsHTMX() *RequestBuilder {
	rb.headers["HX-Request"] = "true"
	return rb
}

// Build creates the http.Request.
func (rb *RequestBuilder) Build() *http.Request {
	req := httptest.NewRequest(rb.method, rb.path, rb.body)
	for k, v := range rb.headers {
		req.Header.Set(k, v)
	}
	return req
}

// Execute runs the request against a handler and returns the response.
func (rb *RequestBuilder) Execute(handler http.Handler) *Response {
	req := rb.Build()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return &Response{
		StatusCode: w.Code,
		Headers:    w.Header(),
		Body:       w.Body.Bytes(),
	}
}

// AssertPanics asserts that the given function panics.
func AssertPanics(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected function to panic")
		}
	}()
	f()
}

// AssertNoPanic asserts that the given function does not panic.
func AssertNoPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()
	f()
}

// BodyReader is a helper to create readers from strings for request bodies.
func BodyReader(s string) io.Reader {
	return bytes.NewReader([]byte(s))
}
