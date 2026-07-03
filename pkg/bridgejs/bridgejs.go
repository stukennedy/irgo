// Package bridgejs embeds the Irgo JS bridge (virtual HTTP, WebSocket, and
// native-capability glue for the WebView). The router serves it automatically
// at GET /_irgo/bridge.js, so app layouts just include:
//
//	<script src="/_irgo/bridge.js"></script>
//
// before Datastar.
package bridgejs

import (
	_ "embed"
	"net/http"
	"strconv"
)

//go:embed irgo-bridge.js
var script []byte

// Script returns the bridge JavaScript source.
func Script() []byte {
	return script
}

// Handler serves the bridge script.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(script)))
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(script)
}
