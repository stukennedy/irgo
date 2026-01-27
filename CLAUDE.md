# Irgo Framework - LLM Reference

This document provides a comprehensive reference for LLMs (Claude, GPT, etc.) working with the Irgo framework.

## Framework Overview

Irgo is a **hypermedia-driven application framework** for building cross-platform apps (iOS, Android, desktop, web) using Go + Datastar + Templ. It follows the hypermedia architecture where the server returns HTML fragments via SSE (Server-Sent Events), not JSON.

### Core Concept

```
User Interaction → Datastar Request → Go Handler → Templ Template → SSE Response → DOM Update
```

### Datastar Overview

[Datastar](https://data-star.dev) is a lightweight (~11KB) hypermedia framework that uses:
- **SSE (Server-Sent Events)** for server responses
- **Reactive signals** for client-side state
- **`data-*` attributes** for declarative behavior

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
│   └── js/datastar.js   # Datastar library
└── mobile/
    └── mobile.go        # Mobile bridge (optional)
```

## Key Packages

### `github.com/stukennedy/irgo/pkg/router`

Chi-based router with Datastar SSE conveniences.

```go
import "github.com/stukennedy/irgo/pkg/router"

r := router.New()

// Standard handlers return (string, error) for HTML responses
r.GET("/path", func(ctx *router.Context) (string, error) {
    return "<div>HTML</div>", nil
})

// Datastar SSE handlers return error only, use ctx.SSE() for responses
r.DSGet("/path", func(ctx *router.Context) error {
    sse := ctx.SSE()
    return sse.PatchTempl(templates.MyComponent())
})

r.DSPost("/path", handler)
r.DSPut("/path", handler)
r.DSPatch("/path", handler)
r.DSDelete("/path", handler)

// URL parameters
r.DSGet("/users/{id}", func(ctx *router.Context) error {
    id := ctx.Param("id")
    // ...
    return nil
})

// Route groups
r.Route("/api", func(r *router.Router) {
    r.DSGet("/users", listUsers)
})

// Static files
r.Static("/static", http.Dir("static"))

// Get the http.Handler
handler := r.Handler()
```

### `router.Context` - Request/Response Helpers

```go
// Standard handler (returns HTML string)
func handler(ctx *router.Context) (string, error) {
    // Input
    ctx.Param("id")           // URL path parameter
    ctx.Query("q")            // Query string parameter
    ctx.FormValue("name")     // Form field value
    ctx.Header("X-Custom")    // Request header

    // Datastar detection
    ctx.IsDatastar()          // true if Accept: text/event-stream

    // Output - HTML responses (for full page loads)
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

    // Output - Redirects
    ctx.Redirect("/new-url")

    // Output - No content
    ctx.NoContent()

    return "<div>response</div>", nil
}

// Datastar SSE handler (returns error only)
func sseHandler(ctx *router.Context) error {
    // Read signals from request body
    var signals struct {
        Name string `json:"name"`
        Page int    `json:"page"`
    }
    ctx.ReadSignals(&signals)

    // Get SSE writer
    sse := ctx.SSE()

    // Patch HTML into DOM (morphs elements by ID)
    sse.PatchTempl(templates.MyComponent(data))
    sse.PatchHTML(`<div id="result">Updated</div>`)

    // Update client-side signals
    sse.PatchSignals(map[string]any{"count": 5})

    // Remove elements
    sse.Remove("#old-element")

    // Redirect browser
    sse.Redirect("/new-url")

    return nil
}
```

### `github.com/stukennedy/irgo/pkg/datastar`

SSE wrapper for Datastar responses with templ integration.

```go
import "github.com/stukennedy/irgo/pkg/datastar"

// Create SSE writer manually (usually use ctx.SSE() instead)
sse := datastar.NewSSE(w, r)

// Patch operations (morph DOM elements)
sse.PatchTempl(templates.Component())           // Render templ and patch
sse.PatchHTML(`<div id="x">HTML</div>`)         // Patch raw HTML

// With options
sse.PatchTempl(comp, datastar.WithModeOuter)    // Replace entire element
sse.PatchTempl(comp, datastar.WithModeAppend)   // Append to element

// Update client signals
sse.PatchSignals(map[string]any{
    "count": 10,
    "name": "John",
})

// Remove elements by selector
sse.Remove("#element-id")
sse.Remove(".class-name")

// Browser navigation
sse.Redirect("/new-page")

// Read signals from request
var signals MyStruct
datastar.ReadSignals(r, &signals)
```

### `github.com/stukennedy/irgo/pkg/render`

Templ template rendering.

```go
import "github.com/stukennedy/irgo/pkg/render"

renderer := render.NewTemplRenderer()

// Render a templ component to string
html, err := renderer.Render(templates.MyComponent(data))
```

### `github.com/stukennedy/irgo/desktop`

Desktop application support (webview + HTTP server + native menus).

```go
import "github.com/stukennedy/irgo/desktop"

// Configuration
config := desktop.Config{
    Title:     "App Name",
    Width:     1024,
    Height:    768,
    Resizable: true,
    Debug:     false,  // Enable browser devtools
    Port:      0,      // 0 = auto-select
    Version:   "1.0.0", // Shown in About menu (macOS)
    SetupMenu: true,    // Setup native menu bar (macOS)
}

// Or use defaults
config := desktop.DefaultConfig()

// Create app with HTTP handler
app := desktop.New(httpHandler, config)

// Run (blocks until window closed)
// On macOS, this sets up the native menu bar automatically if SetupMenu is true
err := app.Run()

// Utilities
staticDir := desktop.FindStaticDir()      // Find static files
resourcePath := desktop.FindResourcePath() // Find bundled resources

// Manual menu setup (if SetupMenu is false)
desktop.SetupMenu("App Name", "1.0.0")
```

#### Native macOS Menu

When `SetupMenu: true` (default), the app automatically creates standard macOS menus:
- **App Menu**: About, Hide, Hide Others, Show All, Quit
- **Edit Menu**: Undo, Redo, Cut, Copy, Paste, Select All
- **Window Menu**: Minimize, Zoom, Bring All to Front

This uses CGO with Objective-C to call Cocoa APIs. On non-macOS platforms, `SetupMenu` is a no-op.

### `github.com/stukennedy/irgo/mobile`

Mobile bridge for iOS/Android.

```go
import "github.com/stukennedy/irgo/mobile"

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

### Datastar Patterns in Templ

#### Signals (Client-Side State)

```go
// Initialize signals with data-signals
templ Counter() {
    <div data-signals="{count: 0}">
        <span data-text="$count">0</span>
        <button data-on:click="$count++">+</button>
    </div>
}

// Two-way binding with data-bind
templ SearchForm() {
    <div data-signals="{query: '', results: []}">
        <input
            type="text"
            data-bind:query
            placeholder="Search..."
        />
        <span data-text="$query.length + ' characters'"></span>
    </div>
}
```

#### Server Requests

```go
// GET request
templ LoadButton() {
    <button data-on:click="@get('/data')">
        Load Data
    </button>
    <div id="result"></div>
}

// POST request with form
templ TodoForm() {
    <div data-signals="{title: ''}">
        <input type="text" data-bind:title placeholder="New todo"/>
        <button data-on:click="@post('/todos')">Add</button>
    </div>
    <ul id="todo-list"></ul>
}

// PUT/PATCH/DELETE
templ TodoItem(todo Todo) {
    <li id={ fmt.Sprintf("todo-%d", todo.ID) }>
        <input
            type="checkbox"
            checked?={ todo.Done }
            data-on:click={ fmt.Sprintf("@patch('/todos/%d')", todo.ID) }
        />
        <span>{ todo.Title }</span>
        <button data-on:click={ fmt.Sprintf("@delete('/todos/%d')", todo.ID) }>
            Delete
        </button>
    </li>
}
```

#### Event Modifiers

```go
// Debounce input
templ SearchInput() {
    <input
        type="text"
        data-bind:query
        data-on:input__debounce.300ms="@get('/search')"
        placeholder="Search..."
    />
}

// Prevent default
templ Form() {
    <form data-on:submit__prevent="@post('/submit')">
        <input type="text" data-bind:name />
        <button type="submit">Submit</button>
    </form>
}

// Once (trigger only once)
templ LazyLoad() {
    <div data-on:intersect__once="@get('/lazy-content')">
        Loading...
    </div>
}
```

#### Conditional Display

```go
// Show/hide based on signal
templ Modal() {
    <div data-signals="{showModal: false}">
        <button data-on:click="$showModal = true">Open</button>
        <div data-show="$showModal" class="modal">
            <p>Modal content</p>
            <button data-on:click="$showModal = false">Close</button>
        </div>
    </div>
}

// Dynamic classes
templ TabButton(name string) {
    <button
        data-class:active="$activeTab === '" + name + "'"
        data-on:click={ "$activeTab = '" + name + "'" }
    >
        { name }
    </button>
}
```

#### Loading Indicators

```go
templ LoadButton() {
    <div data-signals="{loading: false}">
        <button
            data-on:click="@get('/slow-endpoint')"
            data-indicator:loading
            data-attr:disabled="$loading"
        >
            <span data-show="!$loading">Load Data</span>
            <span data-show="$loading">Loading...</span>
        </button>
    </div>
}
```

## Common Patterns

### Datastar Handler + Template Pattern

```go
// handlers/todos.go
func Mount(r *router.Router) {
    // List todos (full page)
    r.GET("/", func(ctx *router.Context) (string, error) {
        todos := db.GetTodos()
        return renderer.Render(templates.TodoPage(todos))
    })

    // Get greeting (Datastar SSE)
    r.DSGet("/greeting", func(ctx *router.Context) error {
        var signals struct {
            Name string `json:"name"`
        }
        ctx.ReadSignals(&signals)

        if signals.Name == "" {
            signals.Name = "World"
        }

        sse := ctx.SSE()
        return sse.PatchTempl(templates.Greeting(signals.Name))
    })

    // Create todo
    r.DSPost("/todos", func(ctx *router.Context) error {
        var signals struct {
            Title string `json:"title"`
        }
        ctx.ReadSignals(&signals)

        if signals.Title == "" {
            return ctx.SSE().PatchTempl(templates.Error("Title required"))
        }

        todo := db.CreateTodo(signals.Title)

        sse := ctx.SSE()
        sse.PatchTempl(templates.TodoItem(todo))
        sse.PatchSignals(map[string]any{"title": ""}) // Clear input
        return nil
    })

    // Delete todo
    r.DSDelete("/todos/{id}", func(ctx *router.Context) error {
        id := ctx.Param("id")
        db.DeleteTodo(id)

        sse := ctx.SSE()
        return sse.Remove("#todo-" + id)
    })

    // Toggle todo
    r.DSPatch("/todos/{id}", func(ctx *router.Context) error {
        id := ctx.Param("id")
        todo := db.ToggleTodo(id)

        sse := ctx.SSE()
        return sse.PatchTempl(templates.TodoItem(todo))
    })
}
```

### App Router Setup

```go
// app/app.go
package app

import (
    "myapp/handlers"
    "github.com/stukennedy/irgo/pkg/router"
)

func NewRouter() *router.Router {
    r := router.New()

    // Mount handlers
    handlers.Mount(r)

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
    "github.com/stukennedy/irgo/desktop"
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
    "github.com/stukennedy/irgo/mobile"
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
irgo new myapp           # Create new project
irgo new .               # Initialize in current directory

# Development
irgo dev                 # Web dev server with hot reload
irgo run desktop         # Run as desktop app
irgo run desktop --dev   # Desktop with devtools
irgo run ios --dev       # iOS Simulator with hot reload
irgo run android --dev   # Android Emulator with hot reload

# Production builds
irgo build desktop       # Build desktop for current OS
irgo build desktop macos # Build macOS .app
irgo build desktop windows # Build Windows .exe
irgo build desktop linux # Build Linux binary
irgo build ios           # Build iOS framework
irgo build android       # Build Android AAR

# Production run
irgo run ios             # Build + run iOS
irgo run android         # Build + run Android

# Utilities
irgo templ               # Generate templ files
irgo install-tools       # Install dev dependencies
```

## macOS App Bundling

The framework includes scripts for creating macOS `.app` bundles and DMG installers in `build/macos/`:

### Icon Generation

```bash
# Generate .icns from a 1024x1024 PNG
./build/macos/generate-icns.sh static/icon.png build/macos/icon.icns
```

### App Bundle Creation

```bash
# Create a .app bundle
./build/macos/create-app-bundle.sh \
  --binary dist/myapp \
  --name "My App" \
  --bundle-id com.example.myapp \
  --icon build/macos/icon.icns \
  --static static \
  --version 1.2.3
```

### DMG Creation

```bash
# Create a DMG installer (optionally signed and notarized)
./build/macos/create-dmg.sh \
  --app "dist/My App.app" \
  --name "My App" \
  --version 1.2.3

# With code signing (set environment variables)
export APPLE_DEVELOPER_ID="Developer ID Application: Your Name (TEAMID)"
export APPLE_NOTARY_PROFILE="my-notary-profile"
./build/macos/create-dmg.sh --app "dist/My App.app" --name "My App"
```

### Build Configuration Files

- `build/macos/Info.plist.tmpl` - App bundle metadata template
- `build/macos/entitlements.plist` - Code signing entitlements for hardened runtime

## Build Tags

The framework uses Go build tags to separate platform-specific code:

```go
//go:build !desktop    // Included in mobile/web builds, excluded from desktop
//go:build desktop     // Included only in desktop builds
```

When building:
- `go build .` → uses `main.go` (mobile/web)
- `go build -tags desktop .` → uses `main_desktop.go` (desktop)
- `irgo run desktop` → automatically adds `-tags desktop`

## Dependencies

```go
// go.mod
require (
    github.com/a-h/templ v0.3.977
    github.com/stukennedy/irgo v0.1.0
    github.com/starfederation/datastar-go v1.1.0
)
```

The irgo module includes:
- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/webview/webview_go` - Desktop webview (CGO required)
- `github.com/starfederation/datastar-go` - Datastar SDK

## Datastar Attribute Reference

### Actions

| Attribute | Description | Example |
|-----------|-------------|---------|
| `data-on:click` | Trigger on click | `data-on:click="@get('/data')"` |
| `data-on:submit` | Trigger on form submit | `data-on:submit__prevent="@post('/submit')"` |
| `data-on:input` | Trigger on input | `data-on:input__debounce.300ms="@get('/search')"` |
| `data-on:intersect` | Trigger when visible | `data-on:intersect__once="@get('/lazy')"` |

### Bindings

| Attribute | Description | Example |
|-----------|-------------|---------|
| `data-signals` | Initialize signals | `data-signals="{count: 0, name: ''}"` |
| `data-bind:X` | Two-way binding | `data-bind:name` |
| `data-text` | Text content | `data-text="$count"` |
| `data-attr:X` | Dynamic attribute | `data-attr:disabled="$loading"` |

### Display

| Attribute | Description | Example |
|-----------|-------------|---------|
| `data-show` | Show/hide element | `data-show="$isVisible"` |
| `data-class:X` | Conditional class | `data-class:active="$isActive"` |
| `data-indicator:X` | Loading indicator | `data-indicator:loading` |

### HTTP Methods

| Expression | Description |
|------------|-------------|
| `@get('/url')` | GET request |
| `@post('/url')` | POST request |
| `@put('/url')` | PUT request |
| `@patch('/url')` | PATCH request |
| `@delete('/url')` | DELETE request |

### Event Modifiers

| Modifier | Description | Example |
|----------|-------------|---------|
| `__prevent` | Prevent default | `data-on:submit__prevent` |
| `__stop` | Stop propagation | `data-on:click__stop` |
| `__once` | Trigger once | `data-on:intersect__once` |
| `__debounce.Xms` | Debounce | `data-on:input__debounce.300ms` |
| `__throttle.Xms` | Throttle | `data-on:scroll__throttle.100ms` |

## Important Notes for LLMs

1. **Always use build tags** when creating entry points:
   - `main.go` needs `//go:build !desktop` as first line
   - `main_desktop.go` needs `//go:build desktop` as first line

2. **Datastar handlers use SSE**. Use `r.DSGet()`, `r.DSPost()`, etc. for Datastar endpoints.

3. **Datastar handlers return `error`**. Use `ctx.SSE()` methods to send responses.

4. **Use `ctx.ReadSignals()`** to read client signals from the request body.

5. **Elements must have IDs** for Datastar to patch them. Use `id` attributes.

6. **Desktop requires CGO**. Ensure `CGO_ENABLED=1` for desktop builds.

7. **Templ files compile to Go**. Run `templ generate` after editing `.templ` files.

8. **Static files** go in `static/` directory. Serve via router or http.FileServer.

9. **The router is chi-based** but with a simplified API. Use `ctx.Param()`, `ctx.Query()`, `ctx.FormValue()`.

10. **For real-time features**, Datastar uses built-in SSE streaming.
