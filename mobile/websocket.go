package mobile

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/stukennedy/irgo/pkg/websocket"
)

// WebSocketCallback is implemented by Swift/Kotlin to receive WebSocket messages.
type WebSocketCallback interface {
	// OnMessage is called when Go has a message to send to the WebView.
	// data is a JSON-encoded websocket.Envelope.
	OnMessage(sessionID string, data string)

	// OnClose is called when a WebSocket session is closed.
	OnClose(sessionID string, code int, reason string)

	// OnError is called when an error occurs.
	OnError(sessionID string, errorMsg string)
}

var (
	wsCallback   WebSocketCallback
	wsCallbackMu sync.RWMutex

	// pollChannels stores channels for sessions using polling instead of callbacks.
	pollChannels   = make(map[string]chan string)
	pollChannelsMu sync.RWMutex
)

// SetWebSocketCallback registers the native callback handler for WebSocket messages.
// Called from Swift/Kotlin during initialization.
func SetWebSocketCallback(cb WebSocketCallback) {
	wsCallbackMu.Lock()
	defer wsCallbackMu.Unlock()
	wsCallback = cb
}

// WebSocketConnect creates a new WebSocket session.
// Returns the session ID.
// Called from JavaScript when HTMX creates a WebSocket connection.
func WebSocketConnect(url string) (string, error) {
	hub := GetHub()
	if hub == nil {
		return "", errors.New("bridge not initialized")
	}

	session, err := hub.Connect(url)
	if err != nil {
		return "", err
	}

	// Start goroutine to forward messages from Go to native
	go forwardSessionMessages(session)

	return session.ID, nil
}

// WebSocketConnectWithID creates a session with a specific ID (for reconnection).
func WebSocketConnectWithID(sessionID, url string) error {
	hub := GetHub()
	if hub == nil {
		return errors.New("bridge not initialized")
	}

	session, err := hub.ConnectWithID(sessionID, url)
	if err != nil {
		return err
	}

	go forwardSessionMessages(session)
	return nil
}

// WebSocketSend sends a message from the WebView to Go.
// data is the JSON message from HTMX (websocket.Request format).
// Returns the response envelope as JSON, or empty string if no immediate response.
func WebSocketSend(sessionID string, data string) (string, error) {
	hub := GetHub()
	if hub == nil {
		return "", errors.New("bridge not initialized")
	}

	envelope, err := hub.HandleMessage(sessionID, []byte(data))
	if err != nil {
		return "", err
	}

	if envelope == nil {
		return "", nil
	}

	// Encode response
	respData, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}

	return string(respData), nil
}

// WebSocketClose closes a WebSocket session.
func WebSocketClose(sessionID string) error {
	hub := GetHub()
	if hub == nil {
		return errors.New("bridge not initialized")
	}

	hub.Disconnect(sessionID)

	// Clean up poll channel if exists
	pollChannelsMu.Lock()
	if ch, ok := pollChannels[sessionID]; ok {
		close(ch)
		delete(pollChannels, sessionID)
	}
	pollChannelsMu.Unlock()

	return nil
}

// WebSocketPoll polls for outgoing messages (alternative to callbacks).
// Returns JSON-encoded envelope or empty string if no messages.
// This is useful for platforms where callbacks are difficult.
func WebSocketPoll(sessionID string) string {
	hub := GetHub()
	if hub == nil {
		return ""
	}

	session, ok := hub.GetSession(sessionID)
	if !ok {
		return ""
	}

	select {
	case envelope, ok := <-session.SendChan:
		if !ok {
			return ""
		}
		data, _ := json.Marshal(envelope)
		return string(data)
	default:
		return ""
	}
}

// WebSocketPollBlocking polls with blocking until a message is available.
// timeout is in milliseconds, 0 for no timeout.
func WebSocketPollBlocking(sessionID string, timeoutMs int) string {
	hub := GetHub()
	if hub == nil {
		return ""
	}

	session, ok := hub.GetSession(sessionID)
	if !ok {
		return ""
	}

	// Non-blocking if timeout is 0
	if timeoutMs <= 0 {
		return WebSocketPoll(sessionID)
	}

	// Block until message available
	envelope, ok := <-session.SendChan
	if !ok {
		return ""
	}
	data, _ := json.Marshal(envelope)
	return string(data)
}

// forwardSessionMessages forwards messages from a session to native code.
func forwardSessionMessages(session *websocket.Session) {
	wsCallbackMu.RLock()
	cb := wsCallback
	wsCallbackMu.RUnlock()

	for envelope := range session.SendChan {
		data, err := json.Marshal(envelope)
		if err != nil {
			if cb != nil {
				cb.OnError(session.ID, err.Error())
			}
			continue
		}

		if cb != nil {
			cb.OnMessage(session.ID, string(data))
		} else {
			// If no callback, try poll channel
			pollChannelsMu.RLock()
			ch, ok := pollChannels[session.ID]
			pollChannelsMu.RUnlock()

			if ok {
				select {
				case ch <- string(data):
				default:
					// Channel full, drop message
				}
			}
		}
	}

	// Session closed
	if cb != nil {
		cb.OnClose(session.ID, 1000, "Session closed")
	}
}

// WebSocketBroadcast sends a message to all sessions matching a URL pattern.
func WebSocketBroadcast(urlPattern string, target string, html string) {
	hub := GetHub()
	if hub == nil {
		return
	}

	envelope := websocket.HTMLEnvelope(target, html)
	hub.BroadcastToURL(urlPattern, envelope)
}

// WebSocketBroadcastAll sends a message to all sessions.
func WebSocketBroadcastAll(target string, html string) {
	hub := GetHub()
	if hub == nil {
		return
	}

	envelope := websocket.HTMLEnvelope(target, html)
	hub.Broadcast(envelope)
}

// WebSocketSendToSession sends a message to a specific session.
func WebSocketSendToSession(sessionID string, target string, html string) error {
	hub := GetHub()
	if hub == nil {
		return errors.New("bridge not initialized")
	}

	envelope := websocket.HTMLEnvelope(target, html)
	return hub.Send(sessionID, envelope)
}

// WebSocketSessionCount returns the number of active sessions.
func WebSocketSessionCount() int {
	hub := GetHub()
	if hub == nil {
		return 0
	}
	return hub.SessionCount()
}

// WebSocketSessionCountForURL returns sessions connected to a URL pattern.
func WebSocketSessionCountForURL(urlPattern string) int {
	hub := GetHub()
	if hub == nil {
		return 0
	}
	return len(hub.SessionsForURL(urlPattern))
}
