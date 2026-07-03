package transport

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/stukennedy/irgo/pkg/adapter"
	"github.com/stukennedy/irgo/pkg/core"
	ws "github.com/stukennedy/irgo/pkg/websocket"
)

// InProcessTransport implements Transport using in-memory request handling.
// No network I/O occurs - all requests are processed directly in Go.
// This is used for mobile platforms and can be enabled on desktop for testing.
type InProcessTransport struct {
	adapter *adapter.HTTPAdapter
	wsHub   *ws.Hub
	config  *Config

	handlers       map[string]ChannelHandler
	defaultHandler ChannelHandler
	handlersMu     sync.RWMutex

	running bool
	mu      sync.RWMutex
}

// NewInProcessTransport creates a new in-process transport.
func NewInProcessTransport(handler http.Handler, wsHub *ws.Hub, opts ...Option) *InProcessTransport {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	// Create websocket hub if not provided
	if wsHub == nil {
		wsHub = ws.NewHub()
	}

	return &InProcessTransport{
		adapter:  adapter.NewHTTPAdapter(handler),
		wsHub:    wsHub,
		config:   config,
		handlers: make(map[string]ChannelHandler),
	}
}

// HandleRequest processes a request entirely in-memory using httptest.
func (t *InProcessTransport) HandleRequest(ctx context.Context, req *core.Request) (*core.Response, error) {
	t.mu.RLock()
	if !t.running {
		t.mu.RUnlock()
		return nil, ErrTransportClosed
	}
	t.mu.RUnlock()

	// The adapter handles all the virtual HTTP processing
	return t.adapter.HandleRequest(req), nil
}

// OpenChannel creates a virtual WebSocket session via the Hub.
func (t *InProcessTransport) OpenChannel(ctx context.Context, url string) (Channel, error) {
	t.mu.RLock()
	if !t.running {
		t.mu.RUnlock()
		return nil, ErrTransportClosed
	}
	t.mu.RUnlock()

	// Create session in the hub
	session, err := t.wsHub.Connect(url)
	if err != nil {
		return nil, err
	}

	ch := newInProcessChannel(session, t.config.ChannelBufferSize)

	// This is the client end of the channel: forward server-side sends
	// (session.SendChan) into Receive(), and close the channel when the
	// session closes.
	go func() {
		defer ch.Close()
		for envelope := range session.SendChan {
			ch.deliverMessage(envelopeToMessage(envelope))
		}
	}()

	return ch, nil
}

// RegisterChannelHandler sets the handler for channels matching a URL pattern.
func (t *InProcessTransport) RegisterChannelHandler(pattern string, handler ChannelHandler) {
	t.handlersMu.Lock()
	defer t.handlersMu.Unlock()
	t.handlers[pattern] = handler

	// Register with the websocket hub
	t.wsHub.Handle(pattern, &inProcessHubAdapter{handler: handler, transport: t})
}

// SetDefaultChannelHandler sets the fallback handler.
func (t *InProcessTransport) SetDefaultChannelHandler(handler ChannelHandler) {
	t.handlersMu.Lock()
	defer t.handlersMu.Unlock()
	t.defaultHandler = handler

	t.wsHub.SetDefaultHandler(&inProcessHubAdapter{handler: handler, transport: t})
}

// Start marks the transport as running. No server is started.
func (t *InProcessTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = true
	return nil
}

// Stop marks the transport as stopped and cleans up.
func (t *InProcessTransport) Stop(ctx context.Context) error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	t.running = false
	t.mu.Unlock()

	// Close all websocket sessions
	t.wsHub.Close()

	return nil
}

// Config returns the transport configuration.
func (t *InProcessTransport) Config() *Config {
	return t.config
}

// Hub returns the WebSocket hub for direct access.
func (t *InProcessTransport) Hub() *ws.Hub {
	return t.wsHub
}

// SendToChannel sends a message to a specific channel by session ID.
func (t *InProcessTransport) SendToChannel(sessionID string, msg *Message) error {
	session, ok := t.wsHub.GetSession(sessionID)
	if !ok {
		return ErrChannelClosed
	}
	if !session.Send(messageToEnvelope(msg)) {
		return ErrChannelFull
	}
	return nil
}

// BroadcastToURL sends a message to all channels matching a URL pattern.
func (t *InProcessTransport) BroadcastToURL(urlPattern string, msg *Message) {
	t.wsHub.BroadcastToURL(urlPattern, messageToEnvelope(msg))
}

// Broadcast sends a message to all channels.
func (t *InProcessTransport) Broadcast(msg *Message) {
	t.wsHub.Broadcast(messageToEnvelope(msg))
}

// findHandler finds a handler for the given URL pattern.
func (t *InProcessTransport) findHandler(url string) ChannelHandler {
	t.handlersMu.RLock()
	defer t.handlersMu.RUnlock()

	// Exact match first
	if handler, ok := t.handlers[url]; ok {
		return handler
	}

	// Prefix match
	for pattern, handler := range t.handlers {
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(url, pattern) {
			return handler
		}
	}

	return t.defaultHandler
}

// inProcessHubAdapter adapts ChannelHandler to ws.MessageHandler for the hub.
type inProcessHubAdapter struct {
	handler   ChannelHandler
	transport *InProcessTransport
}

func (a *inProcessHubAdapter) OnConnect(session *ws.Session) error {
	ch := newInProcessChannel(session, a.transport.config.ChannelBufferSize)
	return a.handler.OnConnect(ch)
}

func (a *inProcessHubAdapter) OnMessage(session *ws.Session, req *ws.Request) (*ws.Envelope, error) {
	ch := newInProcessChannel(session, a.transport.config.ChannelBufferSize)
	msg := wsRequestToMessage(req)

	resp, err := a.handler.OnMessage(ch, msg)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	return messageToEnvelope(resp), nil
}

func (a *inProcessHubAdapter) OnClose(session *ws.Session) {
	ch := newInProcessChannel(session, a.transport.config.ChannelBufferSize)
	a.handler.OnClose(ch)
}
