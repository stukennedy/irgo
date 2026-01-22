# Irgo App Development Guide

This app is built with **Irgo**, a hypermedia-driven framework for cross-platform apps using Go + HTMX 4 + Templ.

## Architecture

```
User Interaction → HTMX Request → Go Handler → Templ Template → HTML Response → DOM Update
```

**Key principle:** The server returns HTML fragments, not JSON. HTMX handles DOM updates.

## Project Structure

```
├── main.go              # Mobile/web entry (//go:build !desktop)
├── main_desktop.go      # Desktop entry (//go:build desktop)
├── app/
│   └── app.go           # Router setup and route definitions
├── handlers/
│   └── handlers.go      # HTTP handlers that return HTML
├── templates/
│   ├── layout.templ     # Base HTML layout
│   └── *.templ          # Page and component templates
├── static/
│   ├── css/output.css   # Tailwind CSS (generated)
│   └── js/
│       ├── htmx.min.js  # HTMX 4 library
│       └── hx-ws.js     # WebSocket extension
└── mobile/
    └── mobile.go        # Mobile bridge (optional)
```

## CLI Commands

```bash
irgo dev                 # Web dev server with hot reload
irgo run desktop         # Run as desktop app
irgo run desktop --dev   # Desktop with devtools
irgo run ios --dev       # iOS Simulator
irgo run android --dev   # Android Emulator
irgo templ               # Regenerate templ files
```

## Router & Handlers

Handlers receive a `router.Context` and return `(string, error)`. The string is HTML.

```go
import (
    "github.com/stukennedy/irgo/pkg/router"
    "github.com/stukennedy/irgo/pkg/render"
)

// Register routes in app/app.go or handlers/handlers.go
r.GET("/users/{id}", func(ctx *router.Context) (string, error) {
    id := ctx.Param("id")           // URL path parameter
    query := ctx.Query("q")         // Query string
    form := ctx.FormValue("name")   // Form field

    return renderer.Render(templates.UserProfile(user))
})

r.POST("/users", createUser)
r.PUT("/users/{id}", updateUser)
r.DELETE("/users/{id}", deleteUser)
```

### Context Methods

**Input:**
- `ctx.Param("id")` - URL path parameter
- `ctx.Query("q")` - Query string parameter
- `ctx.FormValue("name")` - Form field value
- `ctx.Header("X-Custom")` - Request header

**HTMX Detection:**
- `ctx.IsHTMX()` - true if HX-Request header present
- `ctx.HXTarget()` - HX-Target header value
- `ctx.HXTrigger()` - HX-Trigger header value

**Output:**
- Return HTML string from handler
- `ctx.Redirect("/path")` - HTMX-aware redirect
- `ctx.NotFound("message")` - 404 response
- `ctx.BadRequest("message")` - 400 response
- `ctx.NoContent()` - 204 response

**HTMX Response Headers:**
- `ctx.PushURL("/path")` - Update browser URL
- `ctx.Trigger("eventName")` - Trigger client-side event
- `ctx.Retarget("#selector")` - Change swap target
- `ctx.Reswap("innerHTML")` - Change swap strategy
- `ctx.Refresh()` - Full page refresh

## Templ Templates

Templ is a type-safe HTML templating language that compiles to Go.

### Basic Syntax

```go
// templates/components.templ
package templates

// Component with parameters
templ UserCard(name string, email string) {
    <div class="card">
        <h2>{ name }</h2>
        <p>{ email }</p>
    </div>
}

// Component with children
templ Card(title string) {
    <div class="card">
        <h3>{ title }</h3>
        { children... }
    </div>
}

// Usage
templ ProfilePage() {
    @Card("Profile") {
        <p>Content goes here</p>
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
    <input type="checkbox" checked?={ checked }/>
}

// Dynamic classes
templ Item(done bool) {
    <span class={ "item", templ.KV("line-through", done) }>Item</span>
}

// Safe URLs
templ Link(url string) {
    <a href={ templ.SafeURL(url) }>Link</a>
}

// Raw HTML (use sparingly)
templ RawContent(html string) {
    @templ.Raw(html)
}
```

### Rendering in Handlers

```go
renderer := render.NewTemplRenderer()

func handler(ctx *router.Context) (string, error) {
    return renderer.Render(templates.MyComponent(data))
}
```

## HTMX 4 Patterns

This project uses **HTMX 4** from `https://four.htmx.org/`. Key differences from HTMX 1.x/2.x:
- Improved extension system
- Better WebSocket support via `hx-ws` extension
- Enhanced event handling

### Core Attributes

```html
<!-- Trigger requests -->
<button hx-get="/data" hx-target="#result">Load</button>
<button hx-post="/submit" hx-target="#result">Submit</button>
<button hx-put="/update" hx-target="#result">Update</button>
<button hx-delete="/remove" hx-target="#result">Delete</button>
<button hx-patch="/partial" hx-target="#result">Patch</button>

<!-- Target element for response -->
hx-target="#id"           <!-- By ID -->
hx-target="this"          <!-- Current element -->
hx-target="closest .card" <!-- Closest ancestor -->
hx-target="next .item"    <!-- Next sibling -->
hx-target="previous div"  <!-- Previous sibling -->

<!-- How to swap content -->
hx-swap="innerHTML"       <!-- Replace inner content (default) -->
hx-swap="outerHTML"       <!-- Replace entire element -->
hx-swap="beforebegin"     <!-- Insert before element -->
hx-swap="afterbegin"      <!-- Insert at start of element -->
hx-swap="beforeend"       <!-- Insert at end of element -->
hx-swap="afterend"        <!-- Insert after element -->
hx-swap="delete"          <!-- Delete the target element -->
hx-swap="none"            <!-- Don't swap anything -->

<!-- Swap modifiers -->
hx-swap="innerHTML swap:500ms"      <!-- Delay swap -->
hx-swap="innerHTML settle:500ms"    <!-- Delay settle -->
hx-swap="innerHTML scroll:top"      <!-- Scroll after swap -->
hx-swap="innerHTML show:top"        <!-- Show element -->
hx-swap="innerHTML transition:true" <!-- Use view transitions -->
```

### Triggers

```html
<!-- Event triggers -->
hx-trigger="click"                    <!-- On click (default for buttons) -->
hx-trigger="change"                   <!-- On change (for inputs) -->
hx-trigger="submit"                   <!-- On form submit -->
hx-trigger="keyup"                    <!-- On keyup -->
hx-trigger="keyup changed delay:300ms" <!-- Debounced, only if changed -->
hx-trigger="load"                     <!-- On element load -->
hx-trigger="revealed"                 <!-- When scrolled into view -->
hx-trigger="intersect"                <!-- Intersection observer -->
hx-trigger="every 5s"                 <!-- Polling -->

<!-- Trigger modifiers -->
hx-trigger="click once"               <!-- Only trigger once -->
hx-trigger="keyup changed"            <!-- Only if value changed -->
hx-trigger="click consume"            <!-- Stop event propagation -->
hx-trigger="click queue:first"        <!-- Queue behavior -->
hx-trigger="click from:body"          <!-- Listen on different element -->
hx-trigger="click target:.child"      <!-- Filter to specific targets -->
```

### Common Patterns

**Click to load:**
```go
templ LoadButton() {
    <button hx-get="/data" hx-target="#result" hx-swap="innerHTML">
        Load Data
    </button>
    <div id="result"></div>
}
```

**Form submission:**
```go
templ TodoForm() {
    <form hx-post="/todos" hx-target="#todo-list" hx-swap="beforeend"
          hx-on::after-request="this.reset()">
        <input type="text" name="title" required/>
        <button type="submit">Add</button>
    </form>
    <div id="todo-list"></div>
}
```

**Delete with confirmation:**
```go
templ DeleteButton(id string) {
    <button
        hx-delete={ "/items/" + id }
        hx-target={ "#item-" + id }
        hx-swap="delete"
        hx-confirm="Are you sure?"
    >Delete</button>
}
```

**Inline editing:**
```go
templ DisplayMode(id, value string) {
    <span id={ "field-" + id }
          hx-get={ "/edit/" + id }
          hx-trigger="click"
          hx-swap="outerHTML"
          class="cursor-pointer hover:bg-gray-100">
        { value }
    </span>
}

templ EditMode(id, value string) {
    <form hx-put={ "/save/" + id }
          hx-target={ "#field-" + id }
          hx-swap="outerHTML">
        <input type="text" name="value" value={ value } autofocus/>
        <button type="submit">Save</button>
    </form>
}
```

**Infinite scroll:**
```go
templ ItemList(items []Item, page int) {
    for i, item := range items {
        if i == len(items)-1 && len(items) > 0 {
            <div hx-get={ fmt.Sprintf("/items?page=%d", page+1) }
                 hx-trigger="revealed"
                 hx-swap="afterend">
                @ItemCard(item)
            </div>
        } else {
            @ItemCard(item)
        }
    }
}
```

**Active search:**
```go
templ SearchBox() {
    <input type="search" name="q"
           hx-get="/search"
           hx-trigger="keyup changed delay:300ms"
           hx-target="#results"
           placeholder="Search..."/>
    <div id="results"></div>
}
```

**Out-of-band updates (update multiple elements):**
```go
templ NewItemWithCount(item Item, count int) {
    <!-- Main response swapped to target -->
    <div id={ "item-" + item.ID }>{ item.Name }</div>

    <!-- OOB: Also update the counter -->
    <span id="item-count" hx-swap-oob="true">{ fmt.Sprintf("%d", count) }</span>
}
```

**Loading indicator:**
```go
templ LoadingButton() {
    <button hx-get="/slow-endpoint" hx-target="#result">
        <span class="htmx-indicator">Loading...</span>
        <span>Click Me</span>
    </button>
}
```

```css
/* In your CSS */
.htmx-indicator { display: none; }
.htmx-request .htmx-indicator { display: inline; }
.htmx-request .htmx-indicator + span { display: none; }
```

### Request Headers & Parameters

```html
<!-- Include additional values -->
hx-include="[name='csrf']"            <!-- Include input by selector -->
hx-include="closest form"             <!-- Include all form fields -->

<!-- Add request parameters -->
hx-vals='{"key": "value"}'            <!-- Static JSON values -->
hx-vals="js:{key: computeValue()}"    <!-- Dynamic JS values -->

<!-- Add request headers -->
hx-headers='{"X-Custom": "value"}'
```

## WebSocket Extension (hx-ws)

For real-time features, use the WebSocket extension included in this project.

### Server Setup

```go
import (
    "github.com/gorilla/websocket"
    "github.com/stukennedy/irgo/pkg/router"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func wsHandler(ctx *router.Context) (string, error) {
    conn, err := upgrader.Upgrade(ctx.Response(), ctx.Request(), nil)
    if err != nil {
        return "", err
    }
    defer conn.Close()

    // Send HTML fragments to update the page
    for {
        // Read message from client (optional)
        _, msg, err := conn.ReadMessage()
        if err != nil {
            break
        }

        // Send HTML fragment back
        html := `<div id="messages" hx-swap-oob="beforeend">
            <p>New message received</p>
        </div>`
        conn.WriteMessage(websocket.TextMessage, []byte(html))
    }
    return "", nil
}

// Register the route
r.GET("/ws", wsHandler)
```

### Client Setup

```go
templ ChatRoom() {
    <!-- Connect to WebSocket -->
    <div hx-ext="hx-ws" hx-ws-connect="/ws">
        <!-- Messages appear here via OOB updates -->
        <div id="messages"></div>

        <!-- Form sends via WebSocket -->
        <form hx-ws-send>
            <input type="text" name="message"/>
            <button type="submit">Send</button>
        </form>
    </div>
}
```

### WebSocket Attributes

```html
hx-ws-connect="/ws"       <!-- Connect to WebSocket endpoint -->
hx-ws-send                <!-- Send form data via WebSocket -->
```

### Sending Updates from Server

The server sends HTML fragments. Use `hx-swap-oob="true"` to update specific elements:

```go
// Update a specific element by ID
html := `<div id="status" hx-swap-oob="true">Connected</div>`
conn.WriteMessage(websocket.TextMessage, []byte(html))

// Append to a list
html := `<div id="messages" hx-swap-oob="beforeend">
    <p>New message</p>
</div>`
conn.WriteMessage(websocket.TextMessage, []byte(html))

// Update multiple elements at once
html := `
<div id="user-count" hx-swap-oob="true">5 users</div>
<div id="last-activity" hx-swap-oob="true">Just now</div>
`
conn.WriteMessage(websocket.TextMessage, []byte(html))
```

## Build Tags

The framework uses Go build tags to separate platform code:

```go
//go:build !desktop    // Mobile/web builds (main.go)
//go:build desktop     // Desktop builds only (main_desktop.go)
```

- `go build .` → uses `main.go` (mobile/web)
- `go build -tags desktop .` → uses `main_desktop.go`
- `irgo run desktop` → automatically adds `-tags desktop`

## Common Handler Patterns

### CRUD Operations

```go
func Mount(r *router.Router, renderer *render.TemplRenderer) {
    // List
    r.GET("/items", func(ctx *router.Context) (string, error) {
        items, _ := db.GetItems()
        return renderer.Render(templates.ItemList(items))
    })

    // Create
    r.POST("/items", func(ctx *router.Context) (string, error) {
        name := ctx.FormValue("name")
        if name == "" {
            return renderer.Render(templates.Error("Name required"))
        }
        item, _ := db.CreateItem(name)
        return renderer.Render(templates.ItemRow(item))
    })

    // Update
    r.PUT("/items/{id}", func(ctx *router.Context) (string, error) {
        id := ctx.Param("id")
        name := ctx.FormValue("name")
        item, _ := db.UpdateItem(id, name)
        return renderer.Render(templates.ItemRow(item))
    })

    // Delete
    r.DELETE("/items/{id}", func(ctx *router.Context) (string, error) {
        id := ctx.Param("id")
        db.DeleteItem(id)
        return "", nil // hx-swap="delete" removes the element
    })
}
```

### Conditional Full Page vs Fragment

```go
r.GET("/page", func(ctx *router.Context) (string, error) {
    data := fetchData()

    if ctx.IsHTMX() {
        // HTMX request: return just the fragment
        return renderer.Render(templates.PageContent(data))
    }
    // Full page load: return with layout
    return renderer.Render(templates.FullPage(data))
})
```

### Validation Errors

```go
r.POST("/register", func(ctx *router.Context) (string, error) {
    email := ctx.FormValue("email")

    if !isValidEmail(email) {
        // Return error message to be swapped into form
        return renderer.Render(templates.FieldError("email", "Invalid email"))
    }

    // Success - trigger event to close modal or redirect
    ctx.Trigger("registration-complete")
    return renderer.Render(templates.SuccessMessage("Registered!"))
})
```

## Tips

1. **Always read files before editing** - understand existing code first
2. **Run `irgo templ`** after modifying `.templ` files to regenerate Go code
3. **Use `irgo dev`** during development for hot reload
4. **Return HTML fragments**, not JSON - this is hypermedia-driven
5. **Use `hx-swap-oob`** to update multiple page elements from one response
6. **Prefer small, focused components** that can be reused and swapped independently
7. **Test in desktop mode** with `irgo run desktop --dev` for browser devtools
