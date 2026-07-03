package adapter

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"sync"

	"github.com/stukennedy/irgo/pkg/core"
)

// ResponseStream receives a response progressively as the handler produces
// it. This is what makes SSE (and any long-lived response) work through the
// virtual HTTP bridge: chunks are delivered as the handler flushes, not
// buffered until it returns.
//
// Callbacks are invoked sequentially from the goroutine running the handler:
// OnResponse exactly once, OnChunk zero or more times, OnComplete exactly
// once (last).
type ResponseStream interface {
	// OnResponse delivers the status code and headers. Called once, before
	// any chunks, as soon as the handler commits them (first write/flush).
	OnResponse(status int, headersJSON string)

	// OnChunk delivers a piece of the response body. For SSE handlers this
	// corresponds to one flush (typically one complete event).
	OnChunk(chunk []byte)

	// OnComplete signals the end of the stream. errorMessage is "" on
	// success. No further callbacks follow.
	OnComplete(errorMessage string)
}

// HandleRequestStream executes the handler and streams the response through
// the given ResponseStream. It blocks until the handler returns or ctx is
// cancelled; callers typically run it on a background goroutine.
//
// Cancelling ctx cancels the *http.Request context, so handlers that watch
// r.Context().Done() (e.g. datastar's SSE.IsClosed) terminate when the
// client goes away — exactly like a real network disconnect.
//
// Panics (from the handler or from request construction) are recovered and
// reported as a completed 500 stream, mirroring net/http's behavior. This
// often runs on a bare goroutine spawned by the mobile bridge, where a
// re-raised panic would kill the whole app process.
func (a *HTTPAdapter) HandleRequestStream(ctx context.Context, req *core.Request, stream ResponseStream) {
	w := &streamWriter{stream: stream, header: make(http.Header)}

	// A panicking handler must still complete the stream, otherwise the
	// native side waits forever.
	defer func() {
		if r := recover(); r != nil {
			if r != http.ErrAbortHandler {
				log.Printf("irgo/adapter: recovered handler panic: %v\n%s", r, debug.Stack())
			}
			w.completeWithPanic(r)
		}
	}()

	// buildHTTPRequest panics on malformed method/URL from the native side;
	// the deferred recover above turns that into a 500 stream too.
	httpReq := buildHTTPRequest(req).WithContext(ctx)

	a.handler.ServeHTTP(w, httpReq)
	w.finish()
}

// streamWriter is an http.ResponseWriter that forwards the response to a
// ResponseStream. Writes are buffered and delivered on Flush (matching how
// SSE producers emit one event per flush) and on handler completion.
type streamWriter struct {
	stream ResponseStream

	mu          sync.Mutex
	header      http.Header
	buf         bytes.Buffer
	status      int
	wroteHeader bool // headers delivered to the stream
	finished    bool
}

func (w *streamWriter) Header() http.Header {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.header
}

func (w *streamWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wroteHeader {
		return
	}
	w.status = status
	w.sendHeaderLocked()
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.sendHeaderLocked()
	}
	return w.buf.Write(p)
}

// Flush implements http.Flusher. Buffered bytes are delivered as one chunk.
func (w *streamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.sendHeaderLocked()
	}
	w.flushBufLocked()
}

func (w *streamWriter) sendHeaderLocked() {
	w.wroteHeader = true
	w.stream.OnResponse(w.status, core.EncodeHeaders(w.header))
}

func (w *streamWriter) flushBufLocked() {
	if w.buf.Len() == 0 {
		return
	}
	chunk := make([]byte, w.buf.Len())
	copy(chunk, w.buf.Bytes())
	w.buf.Reset()
	w.stream.OnChunk(chunk)
}

// finish delivers any remaining bytes and completes the stream.
func (w *streamWriter) finish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.finished = true
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.sendHeaderLocked()
	}
	w.flushBufLocked()
	w.stream.OnComplete("")
}

func (w *streamWriter) completeWithPanic(_ any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.finished = true
	if !w.wroteHeader {
		w.status = http.StatusInternalServerError
		w.sendHeaderLocked()
	}
	w.stream.OnComplete("handler panic")
}
