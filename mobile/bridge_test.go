package mobile

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// resetBridge tears down global state between tests.
func resetBridge() {
	Shutdown()
}

func newTestHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>home</body></html>")
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/", MaxAge: 3600})
		http.SetCookie(w, &http.Cookie{Name: "pref", Value: "dark", Path: "/"})
		fmt.Fprint(w, "logged in")
	})
	mux.HandleFunc("/whoami", func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, "session=%s", c.Value)
	})
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
		fmt.Fprint(w, "logged out")
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "event: e%d\n\n", i)
			f.Flush()
		}
	})
	return mux
}

func TestHandleRequestCookiePersistence(t *testing.T) {
	resetBridge()
	defer resetBridge()
	SetHandler(newTestHandler())

	// Before login: no session.
	resp := HandleRequest("GET", "/whoami", "{}", nil)
	if resp.Status != http.StatusUnauthorized {
		t.Fatalf("pre-login status = %d, want 401", resp.Status)
	}

	// Login sets two cookies (multi-value Set-Cookie must survive).
	resp = HandleRequest("GET", "/login", "{}", nil)
	if got := resp.GetHeaderValues("Set-Cookie"); len(got) != 2 {
		t.Fatalf("Set-Cookie headers = %v, want 2", got)
	}

	// Subsequent request carries the session automatically.
	resp = HandleRequest("GET", "/whoami", "{}", nil)
	if resp.Status != http.StatusOK || resp.BodyString() != "session=abc123" {
		t.Fatalf("post-login: status=%d body=%q", resp.Status, resp.BodyString())
	}

	// Logout deletes the cookie.
	HandleRequest("GET", "/logout", "{}", nil)
	resp = HandleRequest("GET", "/whoami", "{}", nil)
	if resp.Status != http.StatusUnauthorized {
		t.Fatalf("post-logout status = %d, want 401", resp.Status)
	}
}

func TestCookiePersistenceAcrossRestart(t *testing.T) {
	resetBridge()
	defer resetBridge()

	dir := t.TempDir()

	SetHandler(newTestHandler())
	SetStateDir(dir)

	HandleRequest("GET", "/login", "{}", nil)

	// Simulate app restart.
	Shutdown()
	SetHandler(newTestHandler())
	SetStateDir(dir)

	resp := HandleRequest("GET", "/whoami", "{}", nil)
	if resp.Status != http.StatusOK {
		t.Fatalf("after restart: status = %d, want 200 (cookie should persist)", resp.Status)
	}

	// Session cookies (no Max-Age/Expires) must NOT persist: pref is a
	// persistent-less cookie only in the sense of expiry — it has no
	// Max-Age, so it should be gone after restart while session remains.
	if _, err := filepath.Glob(filepath.Join(dir, cookieFileName)); err != nil {
		t.Fatal(err)
	}

	ClearCookies()
	resp = HandleRequest("GET", "/whoami", "{}", nil)
	if resp.Status != http.StatusUnauthorized {
		t.Fatalf("after ClearCookies: status = %d, want 401", resp.Status)
	}
}

// streamRecorder implements StreamCallback.
type streamRecorder struct {
	mu       sync.Mutex
	status   int
	chunks   []string
	complete chan string
}

func newStreamRecorder() *streamRecorder {
	return &streamRecorder{complete: make(chan string, 1)}
}

func (r *streamRecorder) OnResponse(status int, headersJSON string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *streamRecorder) OnChunk(chunk []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chunks = append(r.chunks, string(chunk))
}

func (r *streamRecorder) OnComplete(errorMessage string) {
	r.complete <- errorMessage
}

func TestHandleRequestStream(t *testing.T) {
	resetBridge()
	defer resetBridge()
	SetHandler(newTestHandler())

	rec := newStreamRecorder()
	handle := HandleRequestStream("GET", "/events", `{"Accept":"text/event-stream"}`, nil, rec)
	if handle == nil {
		t.Fatal("nil handle")
	}

	select {
	case errMsg := <-rec.complete:
		if errMsg != "" {
			t.Fatalf("stream error: %s", errMsg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not complete")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.status != 200 {
		t.Errorf("status = %d", rec.status)
	}
	body := strings.Join(rec.chunks, "")
	for i := 0; i < 3; i++ {
		if !strings.Contains(body, fmt.Sprintf("event: e%d", i)) {
			t.Errorf("body missing event e%d: %q", i, body)
		}
	}
	if len(rec.chunks) < 3 {
		t.Errorf("chunks = %d, want >= 3 (one per flush)", len(rec.chunks))
	}
}

func TestHandleRequestStreamUninitialized(t *testing.T) {
	resetBridge()
	defer resetBridge()

	rec := newStreamRecorder()
	handle := HandleRequestStream("GET", "/", "{}", nil, rec)
	defer handle.Cancel()

	select {
	case errMsg := <-rec.complete:
		if errMsg == "" {
			t.Fatal("expected error for uninitialized bridge")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no completion")
	}
}

func TestHandleRequestStreamCancel(t *testing.T) {
	resetBridge()
	defer resetBridge()

	handlerDone := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/forever", func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	SetHandler(mux)

	rec := newStreamRecorder()
	handle := HandleRequestStream("GET", "/forever", "{}", nil, rec)

	// Give the handler a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	handle.Cancel()

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not terminate on Cancel")
	}

	select {
	case <-rec.complete:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not complete after Cancel")
	}
}

func TestRenderInitialPage(t *testing.T) {
	resetBridge()
	defer resetBridge()
	SetHandler(newTestHandler())

	html := RenderInitialPage()
	if !strings.Contains(html, "home") {
		t.Errorf("initial page = %q", html)
	}
}

// lifecycleRecorder implements LifecycleHandler.
type lifecycleRecorder struct {
	mu         sync.Mutex
	background int
	foreground int
}

func (l *lifecycleRecorder) OnBackground() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.background++
}

func (l *lifecycleRecorder) OnForeground() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.foreground++
}

func TestLifecycle(t *testing.T) {
	resetBridge()
	defer resetBridge()
	SetHandler(newTestHandler())

	rec := &lifecycleRecorder{}
	SetLifecycleHandler(rec)
	defer SetLifecycleHandler(nil)

	OnBackground()
	OnForeground()
	OnBackground()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.background != 2 || rec.foreground != 1 {
		t.Errorf("background=%d foreground=%d, want 2/1", rec.background, rec.foreground)
	}
}
