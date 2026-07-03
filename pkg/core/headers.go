package core

import (
	"encoding/json"
	"net/http"
)

// Header values cross the gomobile bridge as a JSON object. To stay
// backward compatible with native code that sends {"Accept": "text/html"},
// each value may be either a single string or an array of strings.
// Multi-valued headers (Cookie, Set-Cookie) round-trip as arrays.

// DecodeHeaders parses a JSON header object into an http.Header.
// Both string and []string values are accepted. Returns an empty
// (non-nil) header on any parse failure.
func DecodeHeaders(s string) http.Header {
	h := make(http.Header)
	if s == "" || s == "{}" {
		return h
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return h
	}
	for k, v := range raw {
		var single string
		if err := json.Unmarshal(v, &single); err == nil {
			h[http.CanonicalHeaderKey(k)] = append(h[http.CanonicalHeaderKey(k)], single)
			continue
		}
		var multi []string
		if err := json.Unmarshal(v, &multi); err == nil {
			h[http.CanonicalHeaderKey(k)] = append(h[http.CanonicalHeaderKey(k)], multi...)
		}
	}
	return h
}

// EncodeHeaders serializes an http.Header to the JSON wire format.
// Single-valued headers encode as plain strings so existing native
// code (and tests) keep working; multi-valued headers encode as arrays.
func EncodeHeaders(h http.Header) string {
	if len(h) == 0 {
		return "{}"
	}
	out := make(map[string]any, len(h))
	for k, values := range h {
		switch len(values) {
		case 0:
			continue
		case 1:
			out[k] = values[0]
		default:
			out[k] = values
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(data)
}
