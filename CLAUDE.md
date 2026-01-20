# GoHTMX Framework - LLM Reference

This document provides a comprehensive reference for LLMs (Claude, GPT, etc.) working with the GoHTMX framework.

## Framework Overview

GoHTMX is a **hypermedia-driven application framework** for building cross-platform apps (iOS, Android, desktop, web) using Go + HTMX + Templ. It follows the hypermedia architecture where the server returns HTML fragments, not JSON.

### Core Concept

```
User Interaction → HTMX Request → Go Handler → Templ Template → HTML Response → DOM Update
```

### Platform Modes

| Mode | Architecture | Entry Point | Build Tag |
|------|-------------|-------------|-----------|
| **Mobile** | Virtual HTTP via gomobile bridge | `main.go` | `!desktop` |
| **Desktop** | Real HTTP server + native webview | `main_desktop.go` | `desktop` |
| **Web/Dev** | Real HTTP server + browser | `main.go` (serve mode) | `!desktop` |

## Project Structure

```
myapp/
├── main.go              # Mobile/web entry (//go:build !desktop)
├── main_desktop.go      # Desktop entry (//go:build desktop)
├── go.mod
├── app/
│   └── app.go           # Router setup
├── handlers/
│   └── handlers.go      # HTTP handlers
├── templates/
│   ├── layout.templ     # Base HTML layout
│   └── *.templ          # Page/component templates
├── static/
│   ├── css/output.css   # Tailwind CSS
│   └── js/htmx.min.js   # HTMX library
└── mobile/
    └── mobile.go        # Mobile bridge (optional)
```

## Key Packages

### `github.com/stukennedy/gohtmx/pkg/router`

Chi-based router with HTMX conveniences.

```go
import "github.com/stukennedy/gohtmx/pkg/router"

r := router.New()

// Fragment handlers return (string, error)
r.GET("/path", func(ctx *router.Context) (string, error) {
    return "<div>HTML</div>", nil
})

r.POST("/path", handler)
r.PUT("/path", handler)
r.DELETE("/path", handler)
r.PATCH("/path", handler)

// URL parameters
r.GET("/users/{id}", func(ctx *router.Context) (string, error) {
    id := ctx.Param("id")
    return "", nil
})

// Route groups
r.Route("/api", func(r *router.Router) {
    r.GET("/users", listUsers)
})

// Static files
r.Static("/static", http.Dir("static"))

// Get the http.Handler
handler := r.Handler()
```

### `router.Context` - Request/Response Helpers

```go
func handler(ctx *router.Context) (string, error) {
    // Input
    ctx.Param("id")           // URL path parameter
    ctx.Query("q")            // Query string parameter
    ctx.FormValue("name")     // Form field value
    ctx.Header("X-Custom")    // Request header
    
    // HTMX detection
    ctx.IsHTMX()              // true if HX-Request header present
    ctx.HXTarget()            // HX-Target header value
    ctx.HXTrigger()           // HX-Trigger header value
    ctx.HXCurrentURL()        // HX-Current-URL header value
    
    // Output - HTML responses
    ctx.HTML("<div>content</div>")
    ctx.HTMLStatus(201, "<div>created</div>")
    
    // Output - JSON responses
    ctx.JSON(data)
    ctx.JSONStatus(201, data)
    
    // Output - Errors
    ctx.Error(err)
    ctx.ErrorStatus(500, "message")
    ctx.NotFound("not found")
    ctx.BadRequest("invalid input")
    
    // Output - Redirects (HTMX-aware)
    ctx.Redirect("/new-url")
    
    // Output - No content
    ctx.NoContent()
    
    // HTMX response headers
    ctx.PushURL("/new-url")           // Update browser URL
    ctx.ReplaceURL("/new-url")        // Replace browser URL
    ctx.Trigger("eventName")          // Trigger client event
    ctx.TriggerAfterSettle("event")   // Trigger after swap settles
    ctx.TriggerAfterSwap("event")     // Trigger after swap
    ctx.Retarget("#selector")         // Change swap target
    ctx.Reswap("innerHTML")           // Change swap strategy
    ctx.Refresh()                     // Full page refresh
    
    return "<div>response</div>", nil
}
```

### `github.com/stukennedy/gohtmx/pkg/render`

Templ template rendering.

```go
import "github.com/stukennedy/gohtmx/pkg/render"

renderer := render.NewTemplRenderer()

// Render a templ component to string
html, err := renderer.Render(templates.MyComponent(data))
```

### `github.com/stukennedy/gohtmx/desktop`

Desktop application support (webview + HTTP server).

```go
import "github.com/stukennedy/gohtmx/desktop"

// Configuration
config := desktop.Config{
    Title:     "App Name",
    Width:     1024,
    Height:    768,
    Resizable: true,
    Debug:     false,  // Enable browser devtools
    Port:      0,      // 0 = auto-select
}

// Or use defaults
config := desktop.DefaultConfig()

// Create app with HTTP handler
app := desktop.New(httpHandler, config)

// Run (blocks until window closed)
err := app.Run()

// Utilities
staticDir := desktop.FindStaticDir()      // Find static files
resourcePath := desktop.FindResourcePath() // Find bundled resources
```

### `github.com/stukennedy/gohtmx/mobile`

Mobile bridge for iOS/Android.

```go
import "github.com/stukennedy/gohtmx/mobile"

mobile.Initialize()
mobile.SetHandler(r.Handler())
```

## Templ Templates

Templ is a type-safe HTML templating language that compiles to Go.

### Basic Syntax

```go
// templates/example.templ
package templates

// Component with parameters
templ UserCard(name string, age int) {
    <div class="card">
        <h2>{ name }</h2>
        <p>Age: { fmt.Sprintf("%d", age) }</p>
    </div>
}

// Component with children
templ Layout(title string) {
    <!DOCTYPE html>
    <html>
        <head><title>{ title }</title></head>
        <body>
            { children... }
        </body>
    </html>
}

// Using layout
templ HomePage() {
    @Layout("Home") {
        <h1>Welcome</h1>
    }
}

// Conditionals
templ Status(active bool) {
    if active {
        <span class="text-green-500">Active</span>
    } else {
        <span class="text-red-500">Inactive</span>
    }
}

// Loops
templ UserList(users []User) {
    <ul>
        for _, user := range users {
            <li>{ user.Name }</li>
        }
    </ul>
}

// Conditional attributes
templ Checkbox(checked bool) {
    <input type="checkbox" checked?={ checked } />
}

// Dynamic classes with templ.KV
templ Item(done bool) {
    <span class={ templ.KV("line-through", done) }>Item</span>
}

// Dynamic attributes
templ Link(url string) {
    <a href={ templ.SafeURL(url) }>Link</a>
}
```

### HTMX Patterns in Templ

```go
// Click to load
templ LoadButton() {
    <button
        hx-get="/data"
        hx-target="#result"
        hx-swap="innerHTML"
    >Load Data</button>
    <div id="result"></div>
}

// Form submission
templ TodoForm() {
    <form
        hx-post="/todos"
        hx-target="#todo-list"
        hx-swap="beforeend"
        hx-on::after-request="this.reset()"
    >
        <input type="text" name="title" required />
        <button type="submit">Add</button>
    </form>
}

// Delete with confirmation
templ DeleteButton(id string) {
    <button
        hx-delete={ "/items/" + id }
        hx-target={ "#item-" + id }
        hx-swap="delete"
        hx-confirm="Are you sure?"
    >Delete</button>
}

// Inline editing
templ EditableField(id, value string) {
    <span
        id={ "field-" + id }
        hx-get={ "/edit/" + id }
        hx-trigger="click"
        hx-swap="outerHTML"
    >{ value }</span>
}

// Polling
templ LiveStatus() {
    <div
        hx-get="/status"
        hx-trigger="every 5s"
        hx-swap="innerHTML"
    >Loading...</div>
}

// Infinite scroll
templ ItemList(items []Item, nextPage int) {
    for i, item := range items {
        if i == len(items)-1 {
            <div
                hx-get={ fmt.Sprintf("/items?page=%d", nextPage) }
                hx-trigger="revealed"
                hx-swap="afterend"
            >
                @ItemCard(item)
            </div>
        } else {
            @ItemCard(item)
        }
    }
}

// Out-of-band updates
templ ItemWithCounter(item Item, count int) {
    <div id={ "item-" + item.ID }>{ item.Name }</div>
    <span id="counter" hx-swap-oob="true">{ fmt.Sprintf("%d", count) }</span>
}
```

## Common Patterns

### Handler + Template Pattern

```go
// handlers/todos.go
func ListTodos(ctx *router.Context) (string, error) {
    todos, err := db.GetTodos()
    if err != nil {
        return renderer.Render(templates.ErrorMessage(err.Error()))
    }
    return renderer.Render(templates.TodoList(todos))
}

func CreateTodo(ctx *router.Context) (string, error) {
    title := ctx.FormValue("title")
    if title == "" {
        ctx.BadRequest("Title required")
        return "", nil
    }
    
    todo, err := db.CreateTodo(title)
    if err != nil {
        return renderer.Render(templates.ErrorMessage(err.Error()))
    }
    
    // Return just the new item - HTMX will append it
    return renderer.Render(templates.TodoItem(todo))
}

func DeleteTodo(ctx *router.Context) (string, error) {
    id := ctx.Param("id")
    if err := db.DeleteTodo(id); err != nil {
        ctx.ErrorStatus(500, err.Error())
        return "", nil
    }
    // Return empty - hx-swap="delete" will remove the element
    return "", nil
}
```

### App Router Setup

```go
// app/app.go
package app

import (
    "myapp/handlers"
    "github.com/stukennedy/gohtmx/pkg/render"
    "github.com/stukennedy/gohtmx/pkg/router"
)

func NewRouter() *router.Router {
    r := router.New()
    renderer := render.NewTemplRenderer()
    
    // Mount handlers
    handlers.Mount(r, renderer)
    
    return r
}
```

### Desktop Entry Point

```go
// main_desktop.go
//go:build desktop

package main

import (
    "flag"
    "fmt"
    "net/http"

    "myapp/app"
    "github.com/stukennedy/gohtmx/desktop"
)

func main() {
    devMode := flag.Bool("dev", false, "Enable devtools")
    flag.Parse()

    r := app.NewRouter()

    mux := http.NewServeMux()
    staticDir := desktop.FindStaticDir()
    mux.Handle("/static/", http.StripPrefix("/static/", 
        http.FileServer(http.Dir(staticDir))))
    mux.Handle("/", r.Handler())

    config := desktop.DefaultConfig()
    config.Title = "My App"
    config.Debug = *devMode

    desktopApp := desktop.New(mux, config)
    
    fmt.Println("Starting app...")
    if err := desktopApp.Run(); err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Mobile Entry Point

```go
// main.go
//go:build !desktop

package main

import (
    "fmt"
    "log"
    "net/http"
    "os"

    "myapp/app"
    "github.com/stukennedy/gohtmx/mobile"
)

func main() {
    if len(os.Args) > 1 && os.Args[1] == "serve" {
        runDevServer()
        return
    }
    initMobile()
}

func initMobile() {
    mobile.Initialize()
    r := app.NewRouter()
    mobile.SetHandler(r.Handler())
    fmt.Println("Mobile app initialized")
}

func runDevServer() {
    r := app.NewRouter()
    mux := http.NewServeMux()
    mux.Handle("/static/", http.StripPrefix("/static/", 
        http.FileServer(http.Dir("static"))))
    mux.Handle("/", r.Handler())
    
    fmt.Println("Dev server at http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## CLI Commands

```bash
# Project creation
gohtmx new myapp           # Create new project
gohtmx new .               # Initialize in current directory

# Development
gohtmx dev                 # Web dev server with hot reload
gohtmx run desktop         # Run as desktop app
gohtmx run desktop --dev   # Desktop with devtools
gohtmx run ios --dev       # iOS Simulator with hot reload
gohtmx run android --dev   # Android Emulator with hot reload

# Production builds
gohtmx build desktop       # Build desktop for current OS
gohtmx build desktop macos # Build macOS .app
gohtmx build desktop windows # Build Windows .exe
gohtmx build desktop linux # Build Linux binary
gohtmx build ios           # Build iOS framework
gohtmx build android       # Build Android AAR

# Production run
gohtmx run ios             # Build + run iOS
gohtmx run android         # Build + run Android

# Utilities
gohtmx templ               # Generate templ files
gohtmx install-tools       # Install dev dependencies
```

## Build Tags

The framework uses Go build tags to separate platform-specific code:

```go
//go:build !desktop    // Included in mobile/web builds, excluded from desktop
//go:build desktop     // Included only in desktop builds
```

When building:
- `go build .` → uses `main.go` (mobile/web)
- `go build -tags desktop .` → uses `main_desktop.go` (desktop)
- `gohtmx run desktop` → automatically adds `-tags desktop`

## Dependencies

```go
// go.mod
require (
    github.com/a-h/templ v0.3.977
    github.com/stukennedy/gohtmx v0.1.0
)
```

The gohtmx module includes:
- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/webview/webview_go` - Desktop webview (CGO required)

## Important Notes for LLMs

1. **Always use build tags** when creating entry points:
   - `main.go` needs `//go:build !desktop` as first line
   - `main_desktop.go` needs `//go:build desktop` as first line

2. **Handlers return HTML strings**, not JSON. Use `renderer.Render(template)`.

3. **HTMX handles DOM updates**. Return HTML fragments, not full pages for partial updates.

4. **Desktop requires CGO**. Ensure `CGO_ENABLED=1` for desktop builds.

5. **Templ files compile to Go**. Run `templ generate` after editing `.templ` files.

6. **Static files** go in `static/` directory. Serve via router or http.FileServer.

7. **The router is chi-based** but with a simplified API. Use `ctx.Param()`, `ctx.Query()`, `ctx.FormValue()`.

8. **For real-time features**, use WebSockets with HTMX's `hx-ws:connect` attribute.
