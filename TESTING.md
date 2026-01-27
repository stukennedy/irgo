# Testing Irgo Applications

Irgo provides a testing utilities package to make testing your applications easy and expressive.

## Quick Start

```go
import (
    "testing"
    
    "myapp/app"
    irgotest "github.com/stukennedy/irgo/pkg/testing"
)

func TestHomePage(t *testing.T) {
    // Create your router
    r := app.NewRouter()
    
    // Create a test client
    client := irgotest.NewClient(r.Handler())
    
    // Make requests and assert responses
    resp := client.Get("/")
    resp.AssertOK(t)
    resp.AssertContains(t, "Welcome")
}
```

## Test Client

The `Client` provides methods for making HTTP requests to your handler:

```go
client := irgotest.NewClient(handler)

// GET request
resp := client.Get("/path")

// POST with form data
resp := client.PostForm("/path", map[string]string{
    "name": "John",
    "email": "john@example.com",
})

// POST with JSON
resp := client.PostJSON("/path", `{"name":"John"}`)

// PUT, PATCH, DELETE
resp := client.Put("/path", body)
resp := client.PutForm("/path", formData)
resp := client.Patch("/path", body)
resp := client.Delete("/path")
```

### Datastar SSE Requests

Test Datastar-specific behavior:

```go
// Make request with Accept: text/event-stream header (SSE)
resp := client.Datastar().Get("/data")

// With specific target
resp := client.DatastarWithTarget("#content").Get("/data")

// Custom headers
resp := client.WithHeader("Accept", "text/event-stream").Get("/path")
```

## Response Assertions

### Status Codes

```go
resp.AssertStatus(t, 200)    // Assert specific status
resp.AssertOK(t)             // Assert 200
resp.AssertCreated(t)        // Assert 201
resp.AssertNoContent(t)      // Assert 204
resp.AssertBadRequest(t)     // Assert 400
resp.AssertNotFound(t)       // Assert 404
resp.AssertRedirect(t)       // Assert 3xx
```

### Body Content

```go
resp.AssertContains(t, "Welcome")           // Body contains string
resp.AssertNotContains(t, "Error")          // Body doesn't contain string
resp.AssertBodyEquals(t, "<div>Exact</div>") // Exact match
resp.AssertContainsAll(t, "Welcome", "Home", "User") // Contains all strings
```

### Headers

```go
resp.AssertHeader(t, "X-Custom", "value")   // Header equals value
resp.AssertHeaderExists(t, "X-Custom")       // Header exists
resp.AssertContentType(t, "text/html")       // Content-Type starts with
resp.AssertHTML(t)                           // Content-Type is text/html
resp.AssertJSON(t)                           // Content-Type is application/json
```

### SSE Response Assertions

```go
resp.AssertSSE(t)                            // Content-Type is text/event-stream
resp.AssertContains(t, "event: datastar-patch")  // Check for SSE event
resp.AssertContains(t, "data: fragment")     // Check for SSE data
```

### HTML Assertions

```go
html := resp.HTML(t)
html.ContainsElement("div", `id="user"`)    // Contains element with attributes
html.ContainsID("user")                      // Contains element with ID
html.ContainsClass("active")                 // Contains class
```

## Request Builder

For more complex requests, use the fluent request builder:

```go
resp := irgotest.NewRequest("POST", "/users").
    WithHeader("Authorization", "Bearer token").
    WithFormBody(map[string]string{"name": "John"}).
    AsDatastar().
    Execute(handler)

resp.AssertCreated(t)
```

## Complete Example

Here's a full example testing a todo app:

```go
package handlers_test

import (
    "testing"
    
    "myapp/app"
    irgotest "github.com/stukennedy/irgo/pkg/testing"
)

func TestTodoHandlers(t *testing.T) {
    r := app.NewRouter()
    client := irgotest.NewClient(r.Handler())

    t.Run("list todos", func(t *testing.T) {
        resp := client.Get("/todos")
        resp.AssertOK(t)
        resp.AssertHTML(t)
        resp.AssertContains(t, "todo-list")
    })

    t.Run("create todo", func(t *testing.T) {
        resp := client.Datastar().PostForm("/todos", map[string]string{
            "title": "New Todo",
        })
        resp.AssertOK(t)
        resp.AssertContains(t, "New Todo")
        resp.HTML(t).ContainsElement("div", "data-on:click")
    })

    t.Run("create todo validation", func(t *testing.T) {
        resp := client.PostForm("/todos", map[string]string{
            "title": "", // Empty title
        })
        resp.AssertBadRequest(t)
        resp.AssertContains(t, "required")
    })

    t.Run("delete todo", func(t *testing.T) {
        // First create a todo
        client.PostForm("/todos", map[string]string{"title": "To Delete"})

        // Then delete it
        resp := client.Datastar().Delete("/todos/1")
        resp.AssertOK(t)
    })

    t.Run("toggle todo", func(t *testing.T) {
        resp := client.Datastar().Post("/todos/1/toggle", nil)
        resp.AssertOK(t)
        resp.HTML(t).ContainsClass("completed")
    })
}
```

## Testing Templates

If you want to test template rendering separately:

```go
import (
    "testing"
    
    "myapp/templates"
    "github.com/stukennedy/irgo/pkg/render"
)

func TestTodoItemTemplate(t *testing.T) {
    renderer := render.NewTemplRenderer()
    
    todo := &templates.Todo{
        ID:    1,
        Title: "Test Todo",
        Done:  false,
    }
    
    html, err := renderer.Render(templates.TodoItem(todo))
    if err != nil {
        t.Fatalf("render failed: %v", err)
    }
    
    if !strings.Contains(html, "Test Todo") {
        t.Error("expected template to contain todo title")
    }
    
    if !strings.Contains(html, `data-on:click="@delete('/todos/1')"`) {
        t.Error("expected template to contain delete button")
    }
}
```

## Integration Tests with Test Server

For full integration tests that test actual HTTP:

```go
func TestIntegration(t *testing.T) {
    r := app.NewRouter()
    
    // Create a real test server
    server := irgotest.NewTestServer(r.Handler())
    defer server.Close()
    
    // Make real HTTP requests
    resp, err := http.Get(server.URL + "/")
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
}
```

## Testing Desktop Apps

Desktop apps can be tested the same way since they use the same HTTP handler:

```go
func TestDesktopApp(t *testing.T) {
    r := app.NewRouter()
    client := irgotest.NewClient(r.Handler())
    
    // All the same tests work
    resp := client.Get("/")
    resp.AssertOK(t)
}
```

## Mock Renderer

For unit testing handlers without rendering templates:

```go
func TestHandlerWithMockRenderer(t *testing.T) {
    mock := &irgotest.MockRenderer{}
    
    // Use mock in your handler
    handler := NewHandler(mock)
    
    // ... test handler ...
    
    mock.AssertRenderedCount(t, 1)
}
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific test
go test -v ./handlers -run TestTodoHandlers

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Best Practices

1. **Test handlers in isolation**: Create a new router for each test to avoid state leakage.

2. **Test both standard and Datastar SSE requests**: Your handlers may behave differently.

   ```go
   t.Run("regular request", func(t *testing.T) {
       resp := client.Get("/page")
       // Full page expected
   })

   t.Run("datastar request", func(t *testing.T) {
       resp := client.Datastar().Get("/page")
       // SSE fragment expected
   })
   ```

3. **Test error cases**: Don't just test happy paths.

   ```go
   t.Run("invalid input", func(t *testing.T) {
       resp := client.PostForm("/users", map[string]string{})
       resp.AssertBadRequest(t)
   })
   ```

4. **Use table-driven tests** for similar test cases:

   ```go
   tests := []struct {
       name   string
       path   string
       status int
   }{
       {"home", "/", 200},
       {"about", "/about", 200},
       {"missing", "/notfound", 404},
   }
   
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           resp := client.Get(tt.path)
           resp.AssertStatus(t, tt.status)
       })
   }
   ```

5. **Test SSE responses** when your handlers return them:

   ```go
   resp := client.Datastar().Delete("/items/1")
   resp.AssertSSE(t)
   resp.AssertContains(t, "datastar-remove")
   ```
