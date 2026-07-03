package mobile

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// cookieJar is a minimal, serializable cookie store for the virtual HTTP
// bridge. WebViews do not run their cookie machinery for custom schemes
// (irgo://), so without this, Set-Cookie — and therefore any session-based
// auth — silently does nothing on mobile.
//
// The app is a single origin, so domain handling is unnecessary; cookies
// are keyed by name+path with RFC 6265 path-prefix matching and expiry.
type cookieJar struct {
	mu      sync.Mutex
	cookies map[string]*storedCookie
	file    string // persistence path; "" = in-memory only
}

type storedCookie struct {
	Name    string    `json:"name"`
	Value   string    `json:"value"`
	Path    string    `json:"path"`
	Expires time.Time `json:"expires,omitzero"` // zero = session cookie (not persisted)
}

func newCookieJar() *cookieJar {
	return &cookieJar{cookies: make(map[string]*storedCookie)}
}

func cookieKey(name, path string) string {
	return name + "|" + path
}

// setFromResponse stores cookies from response headers.
func (j *cookieJar) setFromResponse(headers http.Header) {
	setCookies := (&http.Response{Header: headers}).Cookies()
	if len(setCookies) == 0 {
		return
	}

	// Only rewrite the on-disk jar when a *persistent* cookie actually
	// changed — apps with rolling-session middleware emit Set-Cookie on
	// every response, and per-response disk writes on the request hot path
	// would add latency and battery drain for nothing.
	persistentChanged := false

	j.mu.Lock()
	for _, c := range setCookies {
		path := c.Path
		if path == "" {
			path = "/"
		}
		key := cookieKey(c.Name, path)
		old := j.cookies[key]

		// MaxAge < 0 or an Expires in the past deletes the cookie.
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			delete(j.cookies, key)
			if old != nil && !old.Expires.IsZero() {
				persistentChanged = true
			}
			continue
		}

		sc := &storedCookie{Name: c.Name, Value: c.Value, Path: path}
		if c.MaxAge > 0 {
			sc.Expires = time.Now().Add(time.Duration(c.MaxAge) * time.Second)
		} else if !c.Expires.IsZero() {
			sc.Expires = c.Expires
		}
		j.cookies[key] = sc

		// Value-based change detection only: rolling-session middleware
		// refreshes Max-Age (and therefore Expires) on every response, and
		// the lifecycle saves (OnBackground/Shutdown) capture the freshest
		// deadlines anyway.
		if !sc.Expires.IsZero() && (old == nil || old.Value != sc.Value || old.Expires.IsZero()) {
			persistentChanged = true
		}
	}
	j.mu.Unlock()

	if persistentChanged {
		j.save()
	}
}

// cookieHeader returns the Cookie header value for a request path,
// or "" if no cookies match.
func (j *cookieJar) cookieHeader(reqPath string) string {
	if reqPath == "" {
		reqPath = "/"
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()
	var matched []*storedCookie
	for key, c := range j.cookies {
		if !c.Expires.IsZero() && c.Expires.Before(now) {
			delete(j.cookies, key)
			continue
		}
		if pathMatches(c.Path, reqPath) {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return ""
	}

	// RFC 6265: longer (more specific) paths first.
	sort.Slice(matched, func(a, b int) bool {
		if len(matched[a].Path) != len(matched[b].Path) {
			return len(matched[a].Path) > len(matched[b].Path)
		}
		return matched[a].Name < matched[b].Name
	})

	parts := make([]string, len(matched))
	for i, c := range matched {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}

// pathMatches implements RFC 6265 §5.1.4 path matching.
func pathMatches(cookiePath, reqPath string) bool {
	if cookiePath == reqPath {
		return true
	}
	if !strings.HasPrefix(reqPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") || reqPath[len(cookiePath)] == '/'
}

// setFile enables persistence and loads any previously saved cookies.
func (j *cookieJar) setFile(path string) {
	j.mu.Lock()
	j.file = path

	data, err := os.ReadFile(path)
	if err == nil {
		var saved []*storedCookie
		if json.Unmarshal(data, &saved) == nil {
			now := time.Now()
			for _, c := range saved {
				if c.Expires.IsZero() || c.Expires.Before(now) {
					continue
				}
				j.cookies[cookieKey(c.Name, c.Path)] = c
			}
		}
	}
	j.mu.Unlock()
}

// save writes persistent (non-session, non-expired) cookies to disk.
func (j *cookieJar) save() {
	j.mu.Lock()
	file := j.file
	var persistent []*storedCookie
	if file != "" {
		now := time.Now()
		for _, c := range j.cookies {
			if !c.Expires.IsZero() && c.Expires.After(now) {
				persistent = append(persistent, c)
			}
		}
		sort.Slice(persistent, func(a, b int) bool {
			return cookieKey(persistent[a].Name, persistent[a].Path) < cookieKey(persistent[b].Name, persistent[b].Path)
		})
	}
	j.mu.Unlock()

	if file == "" {
		return
	}
	data, err := json.Marshal(persistent)
	if err != nil {
		return
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, file)
}

// clear removes all cookies (and the persisted file, if any).
func (j *cookieJar) clear() {
	j.mu.Lock()
	j.cookies = make(map[string]*storedCookie)
	file := j.file
	j.mu.Unlock()
	if file != "" {
		_ = os.Remove(file)
	}
}

const cookieFileName = "irgo-cookies.json"

func cookieFilePath(stateDir string) string {
	return filepath.Join(stateDir, cookieFileName)
}
