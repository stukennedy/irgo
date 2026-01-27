package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stukennedy/irgo/pkg/core"
	"github.com/stukennedy/irgo/pkg/router"
	ws "github.com/stukennedy/irgo/pkg/websocket"
)

// LoopbackTransport implements Transport using a real HTTP server on localhost.
// This is the default transport for desktop applications.
type LoopbackTransport struct {
	handler  http.Handler
	wsHub    *ws.Hub
	server   *http.Server
	config   *Config
	upgrader websocket.Upgrader

	handlers       map[string]ChannelHandler
	defaultHandler ChannelHandler
	handlersMu     sync.RWMutex

	running bool
	mu      sync.RWMutex
	wg      sync.WaitGroup
}

// NewLoopbackTransport creates a new loopback transport.
func NewLoopbackTransport(handler http.Handler, wsHub *ws.Hub, opts ...Option) *LoopbackTransport {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	t := &LoopbackTransport{
		handler:  handler,
		wsHub:    wsHub,
		config:   config,
		handlers: make(map[string]ChannelHandler),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Origin validation is handled by middleware
				return true
			},
		},
	}

	return t
}

// HandleRequest makes a real HTTP request to the localhost server.
// This is primarily used for testing; the webview makes direct HTTP requests.
func (t *LoopbackTransport) HandleRequest(ctx context.Context, req *core.Request) (*core.Response, error) {
	t.mu.RLock()
	if !t.running {
		t.mu.RUnlock()
		return nil, ErrTransportClosed
	}
	t.mu.RUnlock()

	url := fmt.Sprintf("http://%s:%d%s", t.config.Address, t.config.Port, req.URL)

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
	if err != nil {
		return nil, err
	}

	// Apply headers
	for k, v := range req.GetHeaders() {
		httpReq.Header.Set(k, v)
	}

	// Add secret header
	if t.config.Secret != "" {
		httpReq.Header.Set("X-Irgo-Secret", t.config.Secret)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result := &core.Response{
		Status: resp.StatusCode,
		Body:   respBody,
	}

	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}
	result.SetHeaders(respHeaders)

	return result, nil
}

// OpenChannel opens a real WebSocket connection to the localhost server.
func (t *LoopbackTransport) OpenChannel(ctx context.Context, url string) (Channel, error) {
	t.mu.RLock()
	if !t.running {
		t.mu.RUnlock()
		return nil, ErrTransportClosed
	}
	t.mu.RUnlock()

	wsURL := fmt.Sprintf("ws://%s:%d%s", t.config.Address, t.config.Port, url)
	if t.config.Secret != "" {
		wsURL += "?secret=" + t.config.Secret
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}

	return newLoopbackChannel(conn, url), nil
}

// RegisterChannelHandler sets the handler for channels matching a URL pattern.
func (t *LoopbackTransport) RegisterChannelHandler(pattern string, handler ChannelHandler) {
	t.handlersMu.Lock()
	defer t.handlersMu.Unlock()
	t.handlers[pattern] = handler

	// Also register with the websocket hub
	if t.wsHub != nil {
		t.wsHub.Handle(pattern, &hubHandlerAdapter{handler: handler})
	}
}

// SetDefaultChannelHandler sets the fallback handler.
func (t *LoopbackTransport) SetDefaultChannelHandler(handler ChannelHandler) {
	t.handlersMu.Lock()
	defer t.handlersMu.Unlock()
	t.defaultHandler = handler

	if t.wsHub != nil {
		t.wsHub.SetDefaultHandler(&hubHandlerAdapter{handler: handler})
	}
}

// Start starts the HTTP server with security middleware.
func (t *LoopbackTransport) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	// Find available port if not specified
	if t.config.Port == 0 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		t.config.Port = listener.Addr().(*net.TCPAddr).Port
		listener.Close()
	}

	// Generate secret if not provided
	if t.config.Secret == "" {
		// Import would be circular, so we generate inline
		secret, err := generateSecret()
		if err != nil {
			return fmt.Errorf("generating secret: %w", err)
		}
		t.config.Secret = secret
	}

	// Set allowed origins to include our own origin
	origin := fmt.Sprintf("http://%s:%d", t.config.Address, t.config.Port)
	if len(t.config.AllowedOrigins) == 0 {
		t.config.AllowedOrigins = []string{origin}
	}

	// Wrap handler with security middleware
	handler := t.handler

	// WebSocket upgrade handler
	handler = t.wrapWithWebSocketHandler(handler)

	// Security middleware (applied in reverse order)
	handler = router.WebSocketSecretMiddleware(t.config.Secret)(handler)
	handler = router.SecretValidationMiddleware(t.config.Secret, []string{"/static/", "/api/"})(handler)
	handler = router.StrictOriginMiddleware(t.config.AllowedOrigins...)(handler)
	handler = router.CORSMiddleware(t.config.AllowedOrigins...)(handler)

	t.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", t.config.Address, t.config.Port),
		Handler: handler,
	}

	// Create a listener first so we know the server is ready
	listener, err := net.Listen("tcp", t.server.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", t.server.Addr, err)
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		if err := t.server.Serve(listener); err != http.ErrServerClosed {
			fmt.Printf("Loopback transport server error: %v\n", err)
		}
	}()

	t.running = true
	return nil
}

// Stop gracefully shuts down the transport.
func (t *LoopbackTransport) Stop(ctx context.Context) error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	t.running = false
	t.mu.Unlock()

	if t.server != nil {
		if err := t.server.Shutdown(ctx); err != nil {
			return err
		}
	}

	if t.wsHub != nil {
		t.wsHub.Close()
	}

	t.wg.Wait()
	return nil
}

// Config returns the transport configuration.
func (t *LoopbackTransport) Config() *Config {
	return t.config
}

// wrapWithWebSocketHandler adds WebSocket upgrade handling to the handler chain.
func (t *LoopbackTransport) wrapWithWebSocketHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a WebSocket upgrade request
		if !isWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Upgrade to WebSocket
		conn, err := t.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Create session in hub
		session, err := t.wsHub.Connect(r.URL.Path)
		if err != nil {
			conn.Close()
			return
		}

		// Start goroutines for reading/writing
		go t.wsWriter(conn, session)
		go t.wsReader(conn, session)
	})
}

func (t *LoopbackTransport) wsWriter(conn *websocket.Conn, session *ws.Session) {
	defer conn.Close()

	for envelope := range session.SendChan {
		data, err := envelope.JSON()
		if err != nil {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

func (t *LoopbackTransport) wsReader(conn *websocket.Conn, session *ws.Session) {
	defer func() {
		t.wsHub.Disconnect(session.ID)
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		envelope, err := t.wsHub.HandleMessage(session.ID, data)
		if err != nil {
			continue
		}
		if envelope != nil {
			session.Send(envelope)
		}
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return r.Header.Get("Upgrade") == "websocket"
}

// generateSecret creates a cryptographically secure random secret.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// hubHandlerAdapter adapts ChannelHandler to ws.MessageHandler.
type hubHandlerAdapter struct {
	handler ChannelHandler
}

func (a *hubHandlerAdapter) OnConnect(session *ws.Session) error {
	ch := &sessionChannelAdapter{session: session}
	return a.handler.OnConnect(ch)
}

func (a *hubHandlerAdapter) OnMessage(session *ws.Session, req *ws.Request) (*ws.Envelope, error) {
	ch := &sessionChannelAdapter{session: session}
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

func (a *hubHandlerAdapter) OnClose(session *ws.Session) {
	ch := &sessionChannelAdapter{session: session}
	a.handler.OnClose(ch)
}

// sessionChannelAdapter adapts ws.Session to Channel.
type sessionChannelAdapter struct {
	session *ws.Session
}

func (a *sessionChannelAdapter) ID() string  { return a.session.ID }
func (a *sessionChannelAdapter) URL() string { return a.session.URL }
func (a *sessionChannelAdapter) Done() <-chan struct{} {
	// Session doesn't expose a done channel, create one
	done := make(chan struct{})
	go func() {
		for range a.session.SendChan {
			// Drain until closed
		}
		close(done)
	}()
	return done
}

func (a *sessionChannelAdapter) Send(msg *Message) error {
	if a.session.IsClosed() {
		return ErrChannelClosed
	}
	if !a.session.Send(messageToEnvelope(msg)) {
		return ErrChannelFull
	}
	return nil
}

func (a *sessionChannelAdapter) Receive() <-chan *Message {
	// Loopback transport receives messages via the hub, not this channel
	return make(chan *Message)
}

func (a *sessionChannelAdapter) Close() error {
	a.session.Close()
	return nil
}

func (a *sessionChannelAdapter) Set(key string, value any) {
	a.session.Set(key, value)
}

func (a *sessionChannelAdapter) Get(key string) (any, bool) {
	return a.session.Get(key)
}

// Conversion helpers

func wsRequestToMessage(req *ws.Request) *Message {
	return &Message{
		Type:    req.Type,
		ID:      req.RequestID,
		Headers: req.Headers,
		Values:  req.Values,
	}
}

func messageToEnvelope(msg *Message) *ws.Envelope {
	return &ws.Envelope{
		Channel:   msg.Channel,
		Format:    msg.Format,
		Target:    msg.Target,
		Swap:      msg.Swap,
		Payload:   string(msg.Payload),
		RequestID: msg.ID,
	}
}
