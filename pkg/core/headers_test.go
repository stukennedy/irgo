package core

import (
	"strings"
	"testing"
)

func TestDecodeHeadersStringValues(t *testing.T) {
	h := DecodeHeaders(`{"Accept":"text/html","X-Custom":"v"}`)
	if h.Get("Accept") != "text/html" || h.Get("X-Custom") != "v" {
		t.Errorf("decoded = %v", h)
	}
}

func TestDecodeHeadersArrayValues(t *testing.T) {
	h := DecodeHeaders(`{"Set-Cookie":["a=1","b=2"],"Content-Type":"text/html"}`)
	if got := h.Values("Set-Cookie"); len(got) != 2 || got[0] != "a=1" || got[1] != "b=2" {
		t.Errorf("Set-Cookie = %v", got)
	}
	if h.Get("Content-Type") != "text/html" {
		t.Errorf("Content-Type = %q", h.Get("Content-Type"))
	}
}

func TestDecodeHeadersInvalid(t *testing.T) {
	for _, s := range []string{"", "{}", "not json", `{"a":1}`} {
		h := DecodeHeaders(s)
		if h == nil {
			t.Errorf("DecodeHeaders(%q) returned nil", s)
		}
	}
}

func TestEncodeHeadersRoundTrip(t *testing.T) {
	r := NewResponse(200)
	r.AddHeader("Set-Cookie", "a=1")
	r.AddHeader("Set-Cookie", "b=2")
	r.SetHeader("Content-Type", "text/html")

	if got := r.GetHeaderValues("Set-Cookie"); len(got) != 2 {
		t.Errorf("Set-Cookie = %v", got)
	}
	// Single-value headers must encode as plain strings for native-side
	// backward compatibility.
	if !strings.Contains(r.Headers, `"Content-Type":"text/html"`) {
		t.Errorf("Headers = %s", r.Headers)
	}
}

func TestRequestMultiValueHeaders(t *testing.T) {
	req := NewRequest("GET", "/")
	req.AddHeader("Cookie", "a=1")
	req.AddHeader("Cookie", "b=2")

	if got := req.GetHeaderValues("Cookie"); len(got) != 2 {
		t.Errorf("Cookie = %v", got)
	}
	// First value wins in the flat map view.
	if req.GetHeaders()["Cookie"] != "a=1" {
		t.Errorf("flat = %v", req.GetHeaders())
	}
}

func TestHeaderCaseInsensitivity(t *testing.T) {
	req := NewRequest("GET", "/")
	req.SetHeader("content-type", "application/json")
	if req.GetHeader("Content-Type") != "application/json" {
		t.Errorf("case-insensitive lookup failed: %s", req.Headers)
	}
}
