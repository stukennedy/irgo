// Package livereload provides server-sent events for browser live reload during development.
package livereload

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Server handles SSE connections for live reload notifications.
type Server struct {
	buildTime int64
	clients   map[chan string]struct{}
	mu        sync.RWMutex
}

// New creates a new livereload server with the current build time.
func New() *Server {
	return &Server{
		buildTime: time.Now().UnixNano(),
		clients:   make(map[chan string]struct{}),
	}
}

// BuildTime returns the server's build timestamp.
func (s *Server) BuildTime() int64 {
	return s.buildTime
}

// Handler returns an http.HandlerFunc for the SSE endpoint.
// Mount this at /dev/livereload for live reload functionality.
func (s *Server) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Create client channel
		clientChan := make(chan string, 1)
		s.mu.Lock()
		s.clients[clientChan] = struct{}{}
		s.mu.Unlock()

		// Clean up on disconnect
		defer func() {
			s.mu.Lock()
			delete(s.clients, clientChan)
			close(clientChan)
			s.mu.Unlock()
		}()

		// Immediately send current build time so client can detect server restart
		fmt.Fprintf(w, "event: buildtime\ndata: %d\n\n", s.buildTime)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Keep connection alive with heartbeats
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-clientChan:
				fmt.Fprintf(w, "event: reload\ndata: %s\n\n", msg)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-ticker.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}
}

// NotifyReload sends a reload signal to all connected clients.
func (s *Server) NotifyReload() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.clients {
		select {
		case ch <- "reload":
		default:
			// Skip if channel is full
		}
	}
}

// Script returns the JavaScript code to enable live reload.
// Include this in your HTML during development.
func Script() string {
	return `<script>
(function() {
  if (typeof window === 'undefined') return;

  var buildTime = null;
  var retryDelay = 1000;
  var maxRetryDelay = 5000;

  function connect() {
    var es = new EventSource('/dev/livereload');

    es.addEventListener('buildtime', function(e) {
      var serverBuildTime = e.data;
      if (buildTime !== null && buildTime !== serverBuildTime) {
        console.log('[livereload] Server restarted, reloading...');
        window.location.reload();
      }
      buildTime = serverBuildTime;
      retryDelay = 1000;
    });

    es.addEventListener('reload', function(e) {
      console.log('[livereload] Reload signal received');
      window.location.reload();
    });

    es.onerror = function() {
      es.close();
      console.log('[livereload] Connection lost, reconnecting in ' + retryDelay + 'ms...');
      setTimeout(connect, retryDelay);
      retryDelay = Math.min(retryDelay * 1.5, maxRetryDelay);
    };
  }

  connect();
})();
</script>`
}
