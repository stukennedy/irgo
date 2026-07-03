package transport

import (
	"context"
	"sync"

	ws "github.com/stukennedy/irgo/pkg/websocket"
)

// InProcessChannel wraps a websocket.Session to implement the Channel interface.
// It provides bidirectional communication without network I/O.
type InProcessChannel struct {
	session  *ws.Session
	incoming chan *Message
	done     chan struct{}

	closed    bool
	closeMu   sync.RWMutex
	closeOnce sync.Once
}

// newInProcessChannel creates a new channel wrapping a websocket session.
func newInProcessChannel(session *ws.Session, bufferSize int) *InProcessChannel {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	ch := &InProcessChannel{
		session:  session,
		incoming: make(chan *Message, bufferSize),
		done:     make(chan struct{}),
	}

	return ch
}

// ID returns the unique session identifier.
func (c *InProcessChannel) ID() string {
	return c.session.ID
}

// URL returns the connection URL.
func (c *InProcessChannel) URL() string {
	return c.session.URL
}

// Send queues a message to be sent to the client.
func (c *InProcessChannel) Send(msg *Message) error {
	c.closeMu.RLock()
	if c.closed || c.session.IsClosed() {
		c.closeMu.RUnlock()
		return ErrChannelClosed
	}
	c.closeMu.RUnlock()

	envelope := messageToEnvelope(msg)
	if !c.session.Send(envelope) {
		return ErrChannelFull
	}
	return nil
}

// Receive returns a channel for incoming messages from the client.
func (c *InProcessChannel) Receive() <-chan *Message {
	return c.incoming
}

// Close gracefully closes the channel.
func (c *InProcessChannel) Close() error {
	c.closeOnce.Do(func() {
		// Closing incoming under the write lock pairs with deliverMessage
		// holding the read lock across its send — no send can race the
		// close (send on a closed channel panics).
		c.closeMu.Lock()
		c.closed = true
		close(c.done)
		close(c.incoming)
		c.closeMu.Unlock()

		c.session.Close()
	})
	return nil
}

// Done returns a channel that's closed when the channel terminates.
func (c *InProcessChannel) Done() <-chan struct{} {
	return c.done
}

// Set stores metadata on the channel.
func (c *InProcessChannel) Set(key string, value any) {
	c.session.Set(key, value)
}

// Get retrieves metadata from the channel.
func (c *InProcessChannel) Get(key string) (any, bool) {
	return c.session.Get(key)
}

// SendStream sends messages from a stream with backpressure handling.
// Implements StreamingChannel interface.
func (c *InProcessChannel) SendStream(ctx context.Context, stream <-chan *Message) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return ErrChannelClosed
		case msg, ok := <-stream:
			if !ok {
				return nil // Stream completed
			}
			if err := c.Send(msg); err != nil {
				return err
			}
		}
	}
}

// SendHTML is a convenience method to send an HTML message to a target.
func (c *InProcessChannel) SendHTML(target, html string) error {
	return c.Send(NewHTMLMessage(target, html))
}

// Reply sends a response matching a specific request ID.
func (c *InProcessChannel) Reply(requestID, html string) error {
	msg := NewHTMLMessage("", html)
	msg.ID = requestID
	return c.Send(msg)
}

// deliverMessage is called internally to deliver messages to the incoming channel.
// The read lock is held across the send so Close cannot close the channel
// mid-send.
func (c *InProcessChannel) deliverMessage(msg *Message) bool {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	if c.closed {
		return false
	}

	select {
	case c.incoming <- msg:
		return true
	default:
		return false // Buffer full
	}
}

// Session returns the underlying websocket session for advanced usage.
func (c *InProcessChannel) Session() *ws.Session {
	return c.session
}

// Verify InProcessChannel implements StreamingChannel
var _ StreamingChannel = (*InProcessChannel)(nil)
