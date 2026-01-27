package transport

import (
	"context"
)

// Channel represents a bidirectional communication channel (WebSocket-like).
// Channels provide real-time, event-driven communication between the
// WebView frontend and Go backend.
type Channel interface {
	// ID returns the unique session identifier for this channel.
	ID() string

	// URL returns the connection URL (e.g., "/ws/chat").
	URL() string

	// Send queues a message to be sent to the client.
	// Returns ErrChannelClosed if the channel is closed.
	// Returns ErrChannelFull if the buffer is full (non-blocking).
	Send(msg *Message) error

	// Receive returns a channel for incoming messages from the client.
	// The channel is closed when the connection terminates.
	Receive() <-chan *Message

	// Close gracefully closes the channel.
	// After Close, Send returns ErrChannelClosed and Receive is closed.
	Close() error

	// Done returns a channel that's closed when the channel terminates.
	// Use this for select statements to detect channel closure.
	Done() <-chan struct{}

	// Set stores metadata on the channel.
	Set(key string, value any)

	// Get retrieves metadata from the channel.
	Get(key string) (any, bool)
}

// StreamingChannel extends Channel with streaming capabilities.
// Implementations should support backpressure via context cancellation.
type StreamingChannel interface {
	Channel

	// SendStream sends messages from a stream with backpressure handling.
	// Blocks until all messages are sent, the stream is closed, or ctx is cancelled.
	SendStream(ctx context.Context, stream <-chan *Message) error
}

// ChannelHandler processes channel lifecycle events and messages.
type ChannelHandler interface {
	// OnConnect is called when a channel is opened.
	// Return an error to reject the connection.
	OnConnect(ch Channel) error

	// OnMessage is called when a message is received from the client.
	// Return a message to send back, or nil for no response.
	OnMessage(ch Channel, msg *Message) (*Message, error)

	// OnClose is called when the channel is closed.
	OnClose(ch Channel)
}

// ChannelHandlerFunc is a function adapter for simple handlers.
// It only handles OnMessage; OnConnect and OnClose are no-ops.
type ChannelHandlerFunc func(ch Channel, msg *Message) (*Message, error)

// OnConnect implements ChannelHandler (no-op).
func (f ChannelHandlerFunc) OnConnect(ch Channel) error {
	return nil
}

// OnMessage implements ChannelHandler.
func (f ChannelHandlerFunc) OnMessage(ch Channel, msg *Message) (*Message, error) {
	return f(ch, msg)
}

// OnClose implements ChannelHandler (no-op).
func (f ChannelHandlerFunc) OnClose(ch Channel) {
}

// Message represents a channel message.
// This aligns with the existing websocket.Envelope and websocket.Request types.
type Message struct {
	// Type indicates the message type: "request", "response", or "event"
	Type string `json:"type,omitempty"`

	// ID is used for request/response correlation (matches request_id in websocket)
	ID string `json:"request_id,omitempty"`

	// Channel is the logical channel for routing (e.g., "ui", "json", "data")
	Channel string `json:"channel,omitempty"`

	// Format indicates the payload format: "html" or "json"
	Format string `json:"format,omitempty"`

	// Target is the DOM selector for HTML swaps
	Target string `json:"target,omitempty"`

	// Swap is the swap strategy (innerHTML, outerHTML, beforeend, etc.)
	Swap string `json:"swap,omitempty"`

	// Payload is the message content
	Payload []byte `json:"payload"`

	// Headers contains request headers
	Headers map[string]string `json:"headers,omitempty"`

	// Values contains form data and hx-vals
	Values map[string]any `json:"values,omitempty"`
}

// NewMessage creates a new message with the given payload.
func NewMessage(payload []byte) *Message {
	return &Message{
		Channel: "ui",
		Format:  "html",
		Payload: payload,
	}
}

// NewHTMLMessage creates an HTML message for a specific target.
func NewHTMLMessage(target, html string) *Message {
	return &Message{
		Channel: "ui",
		Format:  "html",
		Target:  target,
		Payload: []byte(html),
	}
}

// NewJSONMessage creates a JSON message on a custom channel.
func NewJSONMessage(channel string, payload []byte) *Message {
	return &Message{
		Channel: channel,
		Format:  "json",
		Payload: payload,
	}
}

// WithTarget sets the target selector and returns the message.
func (m *Message) WithTarget(target string) *Message {
	m.Target = target
	return m
}

// WithSwap sets the swap strategy and returns the message.
func (m *Message) WithSwap(swap string) *Message {
	m.Swap = swap
	return m
}

// WithID sets the request ID and returns the message.
func (m *Message) WithID(id string) *Message {
	m.ID = id
	return m
}

// PayloadString returns the payload as a string.
func (m *Message) PayloadString() string {
	return string(m.Payload)
}

// GetValue returns a value from the Values map.
func (m *Message) GetValue(key string) any {
	if m.Values == nil {
		return nil
	}
	return m.Values[key]
}

// GetStringValue returns a string value from Values.
func (m *Message) GetStringValue(key string) string {
	if v, ok := m.GetValue(key).(string); ok {
		return v
	}
	return ""
}

// GetHeader returns a header value.
func (m *Message) GetHeader(key string) string {
	if m.Headers == nil {
		return ""
	}
	return m.Headers[key]
}
