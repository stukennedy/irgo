package adapter

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stukennedy/irgo/pkg/core"
)

// recordingStream captures stream callbacks for assertions.
type recordingStream struct {
	mu       sync.Mutex
	status   int
	headers  string
	chunks   [][]byte
	complete bool
	errMsg   string

	// chunkCh signals each OnChunk, letting tests observe progressive
	// delivery while the handler is still running.
	chunkCh chan []byte
}

func newRecordingStream() *recordingStream {
	return &recordingStream{chunkCh: make(chan []byte, 16)}
}

func (s *recordingStream) OnResponse(status int, headersJSON string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.headers = headersJSON
}

func (s *recordingStream) OnChunk(chunk []byte) {
	s.mu.Lock()
	s.chunks = append(s.chunks, chunk)
	s.mu.Unlock()
	s.chunkCh <- chunk
}

func (s *recordingStream) OnComplete(errorMessage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.complete = true
	s.errMsg = errorMessage
}

func (s *recordingStream) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, c := range s.chunks {
		b.Write(c)
	}
	return b.String()
}

func TestHandleRequestStreamBasic(t *testing.T) {
	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("<div>hello</div>"))
	}))

	stream := newRecordingStream()
	adapter.HandleRequestStream(context.Background(), core.NewRequest("GET", "/"), stream)

	if stream.status != 201 {
		t.Errorf("status = %d, want 201", stream.status)
	}
	if !strings.Contains(stream.headers, "text/html") {
		t.Errorf("headers missing content type: %s", stream.headers)
	}
	if got := stream.body(); got != "<div>hello</div>" {
		t.Errorf("body = %q", got)
	}
	if !stream.complete || stream.errMsg != "" {
		t.Errorf("complete=%v errMsg=%q", stream.complete, stream.errMsg)
	}
}

func TestHandleRequestStreamProgressive(t *testing.T) {
	// An SSE-style handler that writes an event, flushes, and waits for the
	// test to observe the chunk before writing the next event. This proves
	// chunks are delivered while the handler is still running.
	proceed := make(chan struct{})

	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		_, _ = w.Write([]byte("event: one\n\n"))
		flusher.Flush()

		<-proceed

		_, _ = w.Write([]byte("event: two\n\n"))
		flusher.Flush()
	}))

	stream := newRecordingStream()
	done := make(chan struct{})
	go func() {
		adapter.HandleRequestStream(context.Background(), core.NewRequest("GET", "/events"), stream)
		close(done)
	}()

	select {
	case chunk := <-stream.chunkCh:
		if string(chunk) != "event: one\n\n" {
			t.Fatalf("first chunk = %q", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first chunk not delivered while handler still running")
	}

	close(proceed)

	select {
	case chunk := <-stream.chunkCh:
		if string(chunk) != "event: two\n\n" {
			t.Fatalf("second chunk = %q", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second chunk not delivered")
	}

	<-done
	if !stream.complete {
		t.Error("stream not completed")
	}
}

func TestHandleRequestStreamCancellation(t *testing.T) {
	// A stream-until-disconnect handler must terminate when the context is
	// cancelled — this is what SSE.IsClosed() relies on.
	handlerDone := make(chan struct{})

	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()

		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte("event: tick\n\n"))
				w.(http.Flusher).Flush()
			}
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	stream := newRecordingStream()

	go adapter.HandleRequestStream(ctx, core.NewRequest("GET", "/ticks"), stream)

	// Wait for at least one tick, then cancel.
	select {
	case <-stream.chunkCh:
	case <-time.After(2 * time.Second):
		t.Fatal("no tick received")
	}
	cancel()

	select {
	case <-handlerDone:
		// Handler observed the cancellation - no goroutine leak.
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not terminate on context cancellation")
	}
}

func TestHandleRequestStreamHandlerPanic(t *testing.T) {
	// A panicking handler must complete the stream with an error — and must
	// NOT re-raise: this often runs on a bare goroutine in the mobile
	// bridge, where an escaped panic would kill the whole app process.
	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	stream := newRecordingStream()
	adapter.HandleRequestStream(context.Background(), core.NewRequest("GET", "/"), stream)

	if !stream.complete || stream.errMsg == "" {
		t.Errorf("complete=%v errMsg=%q, want completed with error", stream.complete, stream.errMsg)
	}
	if stream.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", stream.status)
	}
}

func TestHandleRequestStreamMalformedURL(t *testing.T) {
	// Malformed URLs from the native side make httptest.NewRequest panic;
	// that must surface as a completed error stream, not a process crash.
	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	stream := newRecordingStream()
	adapter.HandleRequestStream(context.Background(), core.NewRequest("GET", "/a%zz"), stream)

	if !stream.complete || stream.errMsg == "" {
		t.Errorf("complete=%v errMsg=%q, want completed with error", stream.complete, stream.errMsg)
	}
}

func TestHandleRequestStreamMultiValueHeaders(t *testing.T) {
	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "a", Value: "1"})
		http.SetCookie(w, &http.Cookie{Name: "b", Value: "2"})
		_, _ = w.Write([]byte("ok"))
	}))

	stream := newRecordingStream()
	adapter.HandleRequestStream(context.Background(), core.NewRequest("GET", "/"), stream)

	h := core.DecodeHeaders(stream.headers)
	if got := h.Values("Set-Cookie"); len(got) != 2 {
		t.Errorf("Set-Cookie values = %v, want 2 cookies", got)
	}
}

func TestHandleRequestBufferedMultiValueHeaders(t *testing.T) {
	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "a", Value: "1"})
		http.SetCookie(w, &http.Cookie{Name: "b", Value: "2"})
		_, _ = w.Write([]byte("ok"))
	}))

	resp := adapter.HandleRequest(core.NewRequest("GET", "/"))
	if got := resp.GetHeaderValues("Set-Cookie"); len(got) != 2 {
		t.Errorf("Set-Cookie values = %v, want 2 cookies", got)
	}
}

func TestHandleRequestStreamRequestHeaders(t *testing.T) {
	adapter := NewHTTPAdapter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		if got := r.Header.Values("Cookie"); len(got) != 2 {
			t.Errorf("Cookie values = %v, want 2", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))

	req := core.NewRequest("GET", "/")
	req.SetHeader("Accept", "text/event-stream")
	req.AddHeader("Cookie", "a=1")
	req.AddHeader("Cookie", "b=2")

	stream := newRecordingStream()
	adapter.HandleRequestStream(context.Background(), req, stream)
	if !stream.complete {
		t.Fatal("not completed")
	}
}
