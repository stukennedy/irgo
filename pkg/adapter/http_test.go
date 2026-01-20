package adapter

import (
	"net/http"
	"testing"

	"github.com/stukennedy/irgo/pkg/core"
)

func TestHTTPAdapterBasicRequest(t *testing.T) {
	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<h1>Hello</h1>"))
	})

	adapter := NewHTTPAdapter(handler)

	// Make a request
	req := core.NewRequest("GET", "/")
	resp := adapter.HandleRequest(req)

	// Check response
	if resp.Status != 200 {
		t.Errorf("expected status 200, got %d", resp.Status)
	}
	if resp.BodyString() != "<h1>Hello</h1>" {
		t.Errorf("expected body <h1>Hello</h1>, got %s", resp.BodyString())
	}
	if ct := resp.GetHeader("Content-Type"); ct != "text/html" {
		t.Errorf("expected Content-Type text/html, got %s", ct)
	}
}

func TestHTTPAdapterPOSTRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		name := r.FormValue("name")
		if name != "test" {
			t.Errorf("expected form value name=test, got %s", name)
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	})

	adapter := NewHTTPAdapter(handler)

	req := core.NewRequest("POST", "/create")
	req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
	req.Body = []byte("name=test")

	resp := adapter.HandleRequest(req)

	if resp.Status != 201 {
		t.Errorf("expected status 201, got %d", resp.Status)
	}
}

func TestHTTPAdapterHTMXHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check HTMX headers were passed through
		if r.Header.Get("HX-Request") != "true" {
			t.Error("expected HX-Request header")
		}
		if r.Header.Get("HX-Target") != "#content" {
			t.Error("expected HX-Target header")
		}

		// Send HTMX response headers
		w.Header().Set("HX-Trigger", "refresh")
		w.Header().Set("HX-Reswap", "outerHTML")
		w.Write([]byte("<div>updated</div>"))
	})

	adapter := NewHTTPAdapter(handler)

	req := core.NewRequest("GET", "/fragment")
	req.SetHeader("HX-Request", "true")
	req.SetHeader("HX-Target", "#content")

	resp := adapter.HandleRequest(req)

	if resp.GetHeader("HX-Trigger") != "refresh" {
		t.Error("expected HX-Trigger header in response")
	}
	if resp.GetHeader("HX-Reswap") != "outerHTML" {
		t.Error("expected HX-Reswap header in response")
	}
}

func TestHTTPAdapterQueryParams(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Error("expected query param page=2")
		}
		if r.URL.Query().Get("q") != "search term" {
			t.Error("expected query param q=search term")
		}
		w.Write([]byte("ok"))
	})

	adapter := NewHTTPAdapter(handler)

	req := core.NewRequest("GET", "/search?page=2&q=search+term")
	resp := adapter.HandleRequest(req)

	if resp.Status != 200 {
		t.Errorf("expected status 200, got %d", resp.Status)
	}
}

func TestHTTPAdapterNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	adapter := NewHTTPAdapter(handler)

	req := core.NewRequest("GET", "/missing")
	resp := adapter.HandleRequest(req)

	if resp.Status != 404 {
		t.Errorf("expected status 404, got %d", resp.Status)
	}
}
