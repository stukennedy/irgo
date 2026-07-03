// Package router provides a Datastar-aware HTTP router built on chi.
package router

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/stukennedy/irgo/pkg/bridgejs"
	"github.com/stukennedy/irgo/pkg/native"
)

// FragmentHandler is a handler function that returns an HTML fragment.
// Use this for initial page loads (non-SSE requests).
// If an error is returned, an error response is automatically generated.
type FragmentHandler func(ctx *Context) (string, error)

// SSEHandler is a handler function for Datastar SSE requests.
// Use ctx.SSE() to stream responses back to the client.
// Handlers should use the SSE methods to patch elements and signals.
type SSEHandler func(ctx *Context) error

// Router wraps chi with hypermedia-specific conventions.
type Router struct {
	mux *chi.Mux

	// framework endpoints (/_irgo/*) are registered lazily on first serve —
	// chi requires all middleware to be added before any route, so they
	// cannot be registered in New().
	framework     bool
	frameworkOnce sync.Once
}

// New creates a new Router with default middleware. The framework endpoints
// GET /_irgo/bridge.js (the JS bridge) and POST /_irgo/native (Go-side
// native-capability fallback for web/desktop) are served automatically.
func New() *Router {
	r := chi.NewRouter()

	// Default middleware
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(DatastarRequestMiddleware)

	return &Router{mux: r, framework: true}
}

// NewWithoutMiddleware creates a Router without default middleware or
// framework endpoints.
func NewWithoutMiddleware() *Router {
	return &Router{mux: chi.NewRouter()}
}

func (r *Router) ensureFrameworkRoutes() {
	if !r.framework {
		return
	}
	r.frameworkOnce.Do(func() {
		r.mux.Get("/_irgo/bridge.js", bridgejs.Handler)
		r.mux.Post("/_irgo/native", native.HTTPHandler)
	})
}

// Handler returns the underlying http.Handler for use with the adapter.
func (r *Router) Handler() http.Handler {
	r.ensureFrameworkRoutes()
	return r.mux
}

// Use adds middleware to the router.
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

// Fragment registers a handler that returns HTML fragments (for initial page loads).
func (r *Router) Fragment(method, pattern string, handler FragmentHandler) {
	r.mux.Method(method, pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := NewContext(w, req)
		html, err := handler(ctx)
		if err != nil {
			ctx.Error(err)
			return
		}
		if !ctx.Written() {
			ctx.HTML(html)
		}
	}))
}

// SSE registers a handler for Datastar SSE requests.
func (r *Router) SSE(method, pattern string, handler SSEHandler) {
	r.mux.Method(method, pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := NewContext(w, req)
		if err := handler(ctx); err != nil {
			// If not yet streaming, we can send an error response
			if !ctx.Written() {
				ctx.Error(err)
			}
			// If already streaming, error was already logged via SSE.ConsoleError
		}
	}))
}

// GET registers a GET handler that returns HTML fragments.
func (r *Router) GET(pattern string, handler FragmentHandler) {
	r.Fragment(http.MethodGet, pattern, handler)
}

// POST registers a POST handler that returns HTML fragments.
func (r *Router) POST(pattern string, handler FragmentHandler) {
	r.Fragment(http.MethodPost, pattern, handler)
}

// PUT registers a PUT handler that returns HTML fragments.
func (r *Router) PUT(pattern string, handler FragmentHandler) {
	r.Fragment(http.MethodPut, pattern, handler)
}

// PATCH registers a PATCH handler that returns HTML fragments.
func (r *Router) PATCH(pattern string, handler FragmentHandler) {
	r.Fragment(http.MethodPatch, pattern, handler)
}

// DELETE registers a DELETE handler that returns HTML fragments.
func (r *Router) DELETE(pattern string, handler FragmentHandler) {
	r.Fragment(http.MethodDelete, pattern, handler)
}

// --- Datastar SSE Handlers ---
// These methods register handlers for Datastar SSE requests.
// Use ctx.SSE() methods to stream DOM patches and signal updates.

// DSGet registers a GET handler for Datastar SSE requests.
func (r *Router) DSGet(pattern string, handler SSEHandler) {
	r.SSE(http.MethodGet, pattern, handler)
}

// DSPost registers a POST handler for Datastar SSE requests.
func (r *Router) DSPost(pattern string, handler SSEHandler) {
	r.SSE(http.MethodPost, pattern, handler)
}

// DSPut registers a PUT handler for Datastar SSE requests.
func (r *Router) DSPut(pattern string, handler SSEHandler) {
	r.SSE(http.MethodPut, pattern, handler)
}

// DSPatch registers a PATCH handler for Datastar SSE requests.
func (r *Router) DSPatch(pattern string, handler SSEHandler) {
	r.SSE(http.MethodPatch, pattern, handler)
}

// DSDelete registers a DELETE handler for Datastar SSE requests.
func (r *Router) DSDelete(pattern string, handler SSEHandler) {
	r.SSE(http.MethodDelete, pattern, handler)
}

// Handle registers a standard http.Handler.
func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}

// HandleFunc registers a standard http.HandlerFunc.
func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
}

// Mount attaches a sub-router at the given pattern.
func (r *Router) Mount(pattern string, handler http.Handler) {
	r.mux.Mount(pattern, handler)
}

// Group creates a new route group with shared middleware.
func (r *Router) Group(fn func(r *Router)) {
	r.mux.Group(func(c chi.Router) {
		// Create sub-router that wraps the chi Router interface
		subRouter := &Router{mux: chi.NewRouter()}
		fn(subRouter)
		// Mount the sub-router's routes
		c.Mount("/", subRouter.mux)
	})
}

// Route creates a new route group at the given pattern.
func (r *Router) Route(pattern string, fn func(r *Router)) {
	r.mux.Route(pattern, func(c chi.Router) {
		subRouter := &Router{mux: c.(*chi.Mux)}
		fn(subRouter)
	})
}

// With adds inline middleware for a route.
func (r *Router) With(middlewares ...func(http.Handler) http.Handler) *Router {
	return &Router{mux: r.mux.With(middlewares...).(*chi.Mux)}
}

// NotFound registers a custom 404 handler.
func (r *Router) NotFound(handler http.HandlerFunc) {
	r.mux.NotFound(handler)
}

// MethodNotAllowed registers a custom 405 handler.
func (r *Router) MethodNotAllowed(handler http.HandlerFunc) {
	r.mux.MethodNotAllowed(handler)
}

// Static serves static files from the given filesystem.
func (r *Router) Static(pattern string, root http.FileSystem) {
	if pattern != "/" && pattern[len(pattern)-1] != '/' {
		r.mux.Get(pattern, http.RedirectHandler(pattern+"/", http.StatusMovedPermanently).ServeHTTP)
		pattern += "/"
	}
	pattern += "*"

	r.mux.Get(pattern, func(w http.ResponseWriter, req *http.Request) {
		rctx := chi.RouteContext(req.Context())
		pathPrefix := pattern[:len(pattern)-1]
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		rctx.URLParams.Add("*", req.URL.Path[len(pathPrefix):])
		fs.ServeHTTP(w, req)
	})
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.ensureFrameworkRoutes()
	r.mux.ServeHTTP(w, req)
}
