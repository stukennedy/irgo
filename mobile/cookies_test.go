package mobile

import (
	"net/http"
	"testing"
	"time"
)

func setCookieHeader(values ...string) http.Header {
	h := make(http.Header)
	for _, v := range values {
		h.Add("Set-Cookie", v)
	}
	return h
}

func TestCookieJarPathMatching(t *testing.T) {
	j := newCookieJar()
	j.setFromResponse(setCookieHeader(
		"root=1; Path=/",
		"api=2; Path=/api",
		"deep=3; Path=/api/v2",
	))

	tests := []struct {
		path string
		want string
	}{
		{"/", "root=1"},
		{"/other", "root=1"},
		{"/api", "api=2; root=1"},
		{"/api/users", "api=2; root=1"},
		{"/apifake", "root=1"}, // /api must not prefix-match /apifake
		{"/api/v2/x", "deep=3; api=2; root=1"},
	}
	for _, tt := range tests {
		if got := j.cookieHeader(tt.path); got != tt.want {
			t.Errorf("cookieHeader(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestCookieJarExpiry(t *testing.T) {
	j := newCookieJar()
	j.setFromResponse(setCookieHeader("temp=1; Max-Age=1", "keep=2"))

	if got := j.cookieHeader("/"); got != "keep=2; temp=1" {
		t.Fatalf("before expiry: %q", got)
	}

	// Force expiry by rewriting the stored deadline.
	j.mu.Lock()
	j.cookies[cookieKey("temp", "/")].Expires = time.Now().Add(-time.Second)
	j.mu.Unlock()

	if got := j.cookieHeader("/"); got != "keep=2" {
		t.Errorf("after expiry: %q, want keep=2", got)
	}
}

func TestCookieJarDeletion(t *testing.T) {
	j := newCookieJar()
	j.setFromResponse(setCookieHeader("session=abc"))
	j.setFromResponse(setCookieHeader("session=; Max-Age=-1"))

	if got := j.cookieHeader("/"); got != "" {
		t.Errorf("after deletion: %q, want empty", got)
	}
}

func TestCookieJarOverwrite(t *testing.T) {
	j := newCookieJar()
	j.setFromResponse(setCookieHeader("session=old"))
	j.setFromResponse(setCookieHeader("session=new"))

	if got := j.cookieHeader("/"); got != "session=new" {
		t.Errorf("after overwrite: %q", got)
	}
}
