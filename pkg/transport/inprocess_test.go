package transport

import (
	"context"
	"net/http"
	"testing"
	"time"

	ws "github.com/stukennedy/irgo/pkg/websocket"
)

type echoChannelHandler struct{}

func (echoChannelHandler) OnConnect(ch Channel) error { return nil }
func (echoChannelHandler) OnMessage(ch Channel, msg *Message) (*Message, error) {
	return NewHTMLMessage("#out", "echo:"+string(msg.Payload)), nil
}
func (echoChannelHandler) OnClose(ch Channel) {}

func newTestTransport(t *testing.T) *InProcessTransport {
	t.Helper()
	tr := NewInProcessTransport(http.NewServeMux(), ws.NewHub())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Stop(context.Background()) })
	return tr
}

// TestInProcessChannelReceive verifies that server-side sends are delivered
// to the client end's Receive() channel.
func TestInProcessChannelReceive(t *testing.T) {
	tr := newTestTransport(t)
	tr.RegisterChannelHandler("/ws/echo", echoChannelHandler{})

	ch, err := tr.OpenChannel(context.Background(), "/ws/echo")
	if err != nil {
		t.Fatal(err)
	}

	if err := tr.SendToChannel(ch.ID(), NewHTMLMessage("#out", "hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ch.Receive():
		if string(msg.Payload) != "hello" {
			t.Errorf("payload = %q", msg.Payload)
		}
		if msg.Target != "#out" {
			t.Errorf("target = %q", msg.Target)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Receive() delivered nothing — server sends are not forwarded")
	}
}

// TestInProcessChannelDone verifies Done() closes when the session closes.
func TestInProcessChannelDone(t *testing.T) {
	tr := newTestTransport(t)
	tr.RegisterChannelHandler("/ws/echo", echoChannelHandler{})

	ch, err := tr.OpenChannel(context.Background(), "/ws/echo")
	if err != nil {
		t.Fatal(err)
	}

	session, ok := tr.Hub().GetSession(ch.ID())
	if !ok {
		t.Fatal("session not found")
	}
	session.Close()

	select {
	case <-ch.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() not closed after session close")
	}
}
