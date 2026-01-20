# GoHTMX

A hypermedia-driven application framework that uses Go as a runtime kernel with HTMX. Build native iOS, Android, and **desktop** apps using Go, HTML, and HTMX - no JavaScript frameworks required.

## Key Features

- **Go-Powered Apps**: Write your backend logic in Go, compile to native mobile frameworks or desktop apps
- **HTMX for Interactivity**: Use HTMX's hypermedia approach instead of complex JavaScript
- **Cross-Platform**: Single codebase for iOS, Android, desktop (macOS, Windows, Linux), and web
- **Virtual HTTP (Mobile)**: No network sockets - requests are intercepted and handled directly by Go
- **Native Webview (Desktop)**: Real HTTP server with native webview window
- **Type-Safe Templates**: Use [templ](https://templ.guide) for compile-time checked HTML templates
- **Hot Reload Development**: Edit Go/templ code and see changes instantly

## Architecture

### Mobile Architecture (iOS/Android)

```
┌─────────────────────────────────────────────────────────────┐
│                      Mobile App                              │
│  ┌───────────────────────────────────────────────────────┐  │
│  │                 WebView (HTMX)                         │  │
│  │  • HTML rendered by Go templates                       │  │
│  │  • HTMX handles interactions via gohtmx:// scheme     │  │
│  └──────────────────────┬────────────────────────────────┘  │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐  │
│  │           Native Bridge (Swift / Kotlin)               │  │
│  │  • Intercepts gohtmx:// requests                       │  │
│  │  • Routes to Go via gomobile                           │  │
│  └──────────────────────┬────────────────────────────────┘  │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐  │
│  │              Go Runtime (gomobile bind)                │  │
│  │  • HTTP router (chi-based)                             │  │
│  │  • Template rendering (templ)                          │  │
│  │  • Business logic                                      │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### Desktop Architecture (macOS/Windows/Linux)

```
┌─────────────────────────────────────────────────────────────┐
│                     Desktop App                              │
│  ┌───────────────────────────────────────────────────────┐  │
│  │            Native Webview Window                       │  │
│  │  (System webview engine - Chromium/WebKit)            │  │
│  │  Navigates to: http://localhost:PORT                   │  │
│  └──────────────────────┬────────────────────────────────┘  │
│                         │                                    │
│  ┌──────────────────────▼────────────────────────────────┐  │
│  │         Go HTTP Server (localhost:PORT)                │  │
│  │  • Page Routes (Templ → HTML)                          │  │
│  │  • API Routes (JSON)                                   │  │
│  │  • WebSocket/SSE Routes                                │  │
│  │  • Static Asset Server (/static/*)                     │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21+
- [templ](https://templ.guide): `go install github.com/a-h/templ/cmd/templ@latest`
- [air](https://github.com/air-verse/air): `go install github.com/air-verse/air@latest`

**For mobile development:**
- [gomobile](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile): `go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`
- [entr](https://github.com/eradman/entr): `brew install entr` (macOS)
- For iOS: Xcode with iOS Simulator
- For Android: Android Studio with SDK and emulator

**For desktop development:**
- CGO enabled (C compiler required)
- macOS: Xcode Command Line Tools (included with Xcode)
- Windows: MinGW-w64 or similar C compiler
- Linux: GCC and WebKit2GTK dev packages (`apt install libwebkit2gtk-4.0-dev`)

### Install GoHTMX CLI

```bash
go install github.com/stukennedy/gohtmx/cmd/gohtmx@latest
```

Or build from source:

```bash
git clone https://github.com/stukennedy/gohtmx.git
cd gohtmx/cmd/gohtmx
go install .
```

### Create a New Project

```bash
gohtmx new myapp
cd myapp
go mod tidy
bun install  # or: npm install
```

### Run as Desktop App

```bash
gohtmx run desktop         # Run as desktop app
gohtmx run desktop --dev   # With devtools enabled
```

### Development with Hot Reload (Web)

```bash
gohtmx dev                 # Start dev server at http://localhost:8080
```

### iOS Development

```bash
gohtmx run ios --dev       # Hot-reload with iOS Simulator
gohtmx run ios             # Production build
```

### Build for Production

```bash
# Desktop
gohtmx build desktop           # Build for current platform
gohtmx build desktop macos     # Build macOS .app bundle
gohtmx build desktop windows   # Build Windows .exe
gohtmx build desktop linux     # Build Linux binary

# Mobile
gohtmx build ios               # Build iOS framework
gohtmx build android           # Build Android AAR
```

## Project Structure

```
myapp/
├── main.go              # Mobile/web entry point (build tag: !desktop)
├── main_desktop.go      # Desktop entry point (build tag: desktop)
├── go.mod               # Go module definition
├── .air.toml            # Air hot reload configuration
├── package.json         # Node dependencies (Tailwind CSS)
├── tailwind.config.js   # Tailwind configuration
│
├── app/
│   └── app.go           # Router setup and app configuration
│
├── handlers/
│   └── handlers.go      # HTTP handlers (business logic)
│
├── templates/
│   ├── layout.templ     # Base HTML layout
│   ├── home.templ       # Home page template
│   └── components.templ # Reusable components
│
├── static/
│   ├── css/
│   │   ├── input.css    # Tailwind source
│   │   └── output.css   # Generated CSS
│   └── js/
│       └── htmx.min.js  # HTMX library
│
├── mobile/
│   └── mobile.go        # Mobile bridge setup
│
├── ios/                 # iOS Xcode project
├── android/             # Android project
│
└── build/
    ├── ios/             # Built iOS framework
    ├── android/         # Built Android AAR
    └── desktop/         # Built desktop apps
        ├── macos/       # macOS .app bundle
        ├── windows/     # Windows .exe
        └── linux/       # Linux binary
```

## Desktop Development

### How Desktop Mode Works

Desktop mode uses a different architecture than mobile:

1. **Real HTTP Server**: A Go HTTP server starts on an auto-selected localhost port
2. **Native Webview**: A native window with an embedded browser engine opens
3. **Standard HTTP**: The webview navigates to the localhost URL - standard HTTP requests

This means your app works identically to the web dev server, but packaged as a native desktop app.

### Desktop Entry Point

Projects include a `main_desktop.go` with build tag `//go:build desktop`:

```go
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
    mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
    mux.Handle("/", r.Handler())

    config := desktop.DefaultConfig()
    config.Title = "My App"
    config.Debug = *devMode

    desktopApp := desktop.New(mux, config)

    fmt.Println("Starting desktop app...")
    if err := desktopApp.Run(); err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

### Desktop Configuration

```go
config := desktop.Config{
    Title:     "My App",      // Window title
    Width:     1024,          // Window width
    Height:    768,           // Window height
    Resizable: true,          // Allow window resize
    Debug:     false,         // Enable browser devtools
    Port:      0,             // 0 = auto-select available port
}
```

### Running Desktop Apps

```bash
# Run directly (compiles and runs)
gohtmx run desktop

# With devtools (for debugging)
gohtmx run desktop --dev
```

### Building Desktop Apps

```bash
# Build for current platform
gohtmx build desktop

# Build for specific platform
gohtmx build desktop macos     # Creates build/desktop/macos/MyApp.app
gohtmx build desktop windows   # Creates build/desktop/windows/MyApp.exe
gohtmx build desktop linux     # Creates build/desktop/linux/MyApp
```

### Desktop vs Mobile: Key Differences

| Aspect | Mobile | Desktop |
|--------|--------|---------|
| HTTP | Virtual (no sockets) | Real localhost server |
| Bridge | gomobile + native code | None (direct HTTP) |
| Entry point | `main.go` | `main_desktop.go` |
| Build tag | `!desktop` | `desktop` |
| CGO | Not required | Required (webview) |

## Writing Handlers

Handlers process requests and return HTML fragments:

```go
// handlers/handlers.go
package handlers

import (
    "myapp/templates"
    "github.com/stukennedy/gohtmx/pkg/render"
    "github.com/stukennedy/gohtmx/pkg/router"
)

func Mount(r *router.Router, renderer *render.TemplRenderer) {
    r.GET("/about", func(ctx *router.Context) (string, error) {
        return renderer.Render(templates.AboutPage())
    })

    r.GET("/users/{id}", func(ctx *router.Context) (string, error) {
        userID := ctx.Param("id")
        user, err := fetchUser(userID)
        if err != nil {
            return renderer.Render(templates.ErrorMessage("User not found"))
        }
        return renderer.Render(templates.UserProfile(user))
    })

    r.POST("/todos", func(ctx *router.Context) (string, error) {
        title := ctx.FormValue("title")
        todo := createTodo(title)
        return renderer.Render(templates.TodoItem(todo))
    })
}
```

## Writing Templates

Templates use [templ](https://templ.guide):

```go
// templates/home.templ
package templates

templ HomePage() {
    @Layout("Home") {
        <main class="container mx-auto p-4">
            <h1 class="text-2xl font-bold">Welcome to GoHTMX</h1>
            <nav hx-boost="true">
                <a href="/about" class="text-blue-500">About</a>
            </nav>
        </main>
    }
}

templ TodoItem(todo Todo) {
    <div id={ "todo-" + todo.ID } class="flex items-center gap-2 p-2">
        <input
            type="checkbox"
            checked?={ todo.Done }
            hx-post={ "/todos/" + todo.ID + "/toggle" }
            hx-target={ "#todo-" + todo.ID }
            hx-swap="outerHTML"
        />
        <span>{ todo.Title }</span>
        <button
            hx-delete={ "/todos/" + todo.ID }
            hx-target={ "#todo-" + todo.ID }
            hx-swap="delete"
            class="text-red-500"
        >Delete</button>
    </div>
}
```

## CLI Commands

```bash
# Create new project
gohtmx new myapp
gohtmx new .              # Initialize in current directory

# Development
gohtmx dev                # Start dev server with hot reload (web)
gohtmx run desktop        # Run as desktop app
gohtmx run desktop --dev  # Desktop with devtools
gohtmx run ios --dev      # Hot reload with iOS Simulator
gohtmx run android --dev  # Hot reload with Android Emulator

# Production builds
gohtmx build desktop      # Build desktop app for current platform
gohtmx build desktop macos/windows/linux  # Cross-platform builds
gohtmx build ios          # Build iOS framework
gohtmx build android      # Build Android AAR
gohtmx build all          # Build all mobile platforms

gohtmx run ios            # Build and run on iOS Simulator
gohtmx run android        # Build and run on Android Emulator

# Utilities
gohtmx templ              # Generate templ files
gohtmx install-tools      # Install required dev tools
gohtmx version            # Print version
gohtmx help [command]     # Show help
```

## WebSocket Support

GoHTMX supports real-time updates via WebSockets using HTMX 4's `hx-ws` extension:

```go
templ LiveDashboard() {
    <div hx-ws:connect="/ws/updates">
        <div id="live-data">Waiting for updates...</div>
    </div>
}
```

See the full WebSocket documentation in [WEBSOCKETS.md](WEBSOCKETS.md).

## Troubleshooting

### Desktop: "CGO_ENABLED=0" error

Desktop builds require CGO. Ensure you have a C compiler:
- macOS: `xcode-select --install`
- Windows: Install MinGW-w64
- Linux: `apt install build-essential`

### Desktop: Webview not showing

Check that WebKit2GTK is installed (Linux):
```bash
apt install libwebkit2gtk-4.0-dev
```

### "Module not found" errors

```bash
go mod tidy
```

### Hot reload not working

1. Check if air is running
2. Verify `.air.toml` configuration
3. Make sure `_templ.go` is NOT in `exclude_regex`

### Port 8080 already in use

```bash
lsof -i :8080
kill <PID>
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Acknowledgments

- [HTMX](https://htmx.org) - The hypermedia approach that makes this possible
- [templ](https://templ.guide) - Type-safe HTML templating for Go
- [chi](https://github.com/go-chi/chi) - Lightweight Go router
- [webview](https://github.com/webview/webview) - Native webview for desktop
- [gomobile](https://pkg.go.dev/golang.org/x/mobile) - Go on mobile platforms
- [air](https://github.com/air-verse/air) - Live reload for Go
