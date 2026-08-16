// Package datastarjs embeds the Datastar client and serves it at
// GET /_irgo/datastar.js.
//
// Datastar has two halves that speak a protocol to each other: the Go library
// that writes SSE events, and the browser client that applies them. They were
// two unrelated dependencies here — the Go side from go.mod, the client
// downloaded from a CDN when a project was scaffolded and then vendored into
// that project forever. Nothing connected them, and datastar-go publishes no
// version constant and embeds no bundle, so nothing could.
//
// The result was a release candidate of the client running against a stable
// v1.1.0 of the library in every generated project, plus a third copy in the
// framework's own static directory, plus a script tag elsewhere pointing at an
// unversioned CDN URL — whatever upstream had published that morning. They
// agreed by luck: the wire format happens not to have changed.
//
// Embedding it here makes the pairing structural. Both halves upgrade together
// with the framework, a project downloads nothing when it is created, and a
// project whose client and library disagree stops being expressible.
package datastarjs

import (
	"bytes"
	_ "embed"
	"net/http"
	"strconv"
)

// Version is the Datastar client vendored here.
//
// It is a matched pair with the github.com/starfederation/datastar-go
// requirement in go.mod. Bump them together, and check the wire format when
// you do: the event names the library writes and the client listens for are
// the contract, and nothing but this comment enforces it.
const Version = "v1.0.2"

//go:embed datastar.js
var embedded []byte

// script is the bundle with its source-map reference removed.
//
// The bundle ends with a //# sourceMappingURL=datastar.js.map comment, and the
// map is not distributed — Datastar does not publish it and it is not vendored
// here. A browser only fetches a map when devtools are open, so the dangling
// reference is invisible in normal use and every irgo project has been serving
// it. It is not invisible to everything: a headless browser that resolves the
// comment eagerly fails to compile the whole client, which is how this was
// found.
var script = stripSourceMap(embedded)

func stripSourceMap(b []byte) []byte {
	const marker = "//# sourceMappingURL="
	i := bytes.LastIndex(b, []byte(marker))
	if i < 0 {
		return b
	}
	// Keep anything after the comment's line, in case it is not last.
	rest := b[i:]
	if nl := bytes.IndexByte(rest, '\n'); nl >= 0 {
		return append(bytes.Clone(b[:i]), rest[nl+1:]...)
	}
	return bytes.TrimRight(b[:i], "\n\r\t ")
}

// Script returns the Datastar client source, for builds that embed it rather
// than serve it — the mobile shells read it through the same bridge as
// everything else.
func Script() []byte { return script }

// Handler serves the client.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(script)))
	// Versioned with the framework, so it can be cached hard — unlike the
	// bridge, which is small and changes with every irgo release.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(script)
}
