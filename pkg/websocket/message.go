// Package websocket provides virtual WebSocket support for real-time communication.
// It enables bidirectional communication between the WebView and Go
// without actual network sockets, complementing Datastar's SSE-based updates.
package websocket

import (
	"encoding/json"
)

// Request represents a message from the client via WebSocket.
// Used for real-time bidirectional communication alongside Datastar's SSE.
type Request struct {
	Type      string            `json:"type"`                 // Always "request" for client messages
	RequestID string            `json:"request_id"`           // Unique ID for request-response matching
	Event     string            `json:"event"`                // DOM event that triggered the send (click, submit, etc.)
	Headers   map[string]string `json:"headers"`              // Request headers
	Values    map[string]any    `json:"values"`               // Form data and hx-vals
	Path      string            `json:"path"`                 // Normalized WebSocket URL
	ID        string            `json:"id,omitempty"`         // Element ID (if element has id attribute)
}

// GetValue returns a value from the Values map.
func (r *Request) GetValue(key string) any {
	return r.Values[key]
}

// GetStringValue returns a string value from Values.
func (r *Request) GetStringValue(key string) string {
	if v, ok := r.Values[key].(string); ok {
		return v
	}
	return ""
}

// GetHeader returns a header value.
func (r *Request) GetHeader(key string) string {
	return r.Headers[key]
}

// Target returns the HX-Target header value.
func (r *Request) Target() string {
	return r.Headers["HX-Target"]
}

// CurrentURL returns the HX-Current-URL header value.
func (r *Request) CurrentURL() string {
	return r.Headers["HX-Current-URL"]
}

// Envelope represents a message from the server to the client.
// Used for WebSocket-based real-time updates.
type Envelope struct {
	Channel   string `json:"channel,omitempty"`    // Channel identifier (default: "ui")
	Format    string `json:"format,omitempty"`     // Message format (default: "html")
	Target    string `json:"target,omitempty"`     // Target selector for swap
	Swap      string `json:"swap,omitempty"`       // Swap strategy (innerHTML, outerHTML, etc.)
	Payload   string `json:"payload"`              // The actual content (HTML for ui/html)
	RequestID string `json:"request_id,omitempty"` // Matches original request for response matching
}

// NewEnvelope creates a new UI/HTML envelope with the given payload.
func NewEnvelope(payload string) *Envelope {
	return &Envelope{
		Channel: "ui",
		Format:  "html",
		Payload: payload,
	}
}

// WithTarget sets the target selector.
func (e *Envelope) WithTarget(target string) *Envelope {
	e.Target = target
	return e
}

// WithSwap sets the swap strategy.
func (e *Envelope) WithSwap(swap string) *Envelope {
	e.Swap = swap
	return e
}

// WithRequestID sets the request ID for response matching.
func (e *Envelope) WithRequestID(id string) *Envelope {
	e.RequestID = id
	return e
}

// AsJSON sets the envelope to use JSON format instead of HTML.
func (e *Envelope) AsJSON() *Envelope {
	e.Format = "json"
	return e
}

// ToChannel sets a custom channel for non-UI messages.
func (e *Envelope) ToChannel(channel string) *Envelope {
	e.Channel = channel
	return e
}

// JSON encodes the envelope to JSON bytes.
func (e *Envelope) JSON() ([]byte, error) {
	return json.Marshal(e)
}

// MustJSON encodes the envelope to JSON string, panics on error.
func (e *Envelope) MustJSON() string {
	data, err := e.JSON()
	if err != nil {
		panic(err)
	}
	return string(data)
}

// ParseRequest parses a JSON message into a Request.
func ParseRequest(data []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// HTMLEnvelope creates an envelope for HTML content targeting a specific element.
func HTMLEnvelope(target, html string) *Envelope {
	return &Envelope{
		Channel: "ui",
		Format:  "html",
		Target:  target,
		Payload: html,
	}
}

// SwapEnvelope creates an envelope with a specific swap strategy.
func SwapEnvelope(target, swap, html string) *Envelope {
	return &Envelope{
		Channel: "ui",
		Format:  "html",
		Target:  target,
		Swap:    swap,
		Payload: html,
	}
}

// ReplyEnvelope creates an envelope that replies to a specific request.
func ReplyEnvelope(requestID, html string) *Envelope {
	return &Envelope{
		Channel:   "ui",
		Format:    "html",
		Payload:   html,
		RequestID: requestID,
	}
}

// JSONEnvelope creates an envelope for JSON data on a custom channel.
func JSONEnvelope(channel string, data any) (*Envelope, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Channel: channel,
		Format:  "json",
		Payload: string(payload),
	}, nil
}
