package websocket

import (
	"sync"
	"testing"
)

// TestSendCloseRace exercises the Send/Close race that previously could
// panic with "send on closed channel". Run with -race.
func TestSendCloseRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		s := NewSession("s1", "/ws/test", nil)

		var wg sync.WaitGroup
		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					s.Send(NewEnvelope("payload"))
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Close()
		}()
		wg.Wait()
	}
}

func TestSendAfterClose(t *testing.T) {
	s := NewSession("s1", "/ws/test", nil)
	s.Close()
	if s.Send(NewEnvelope("x")) {
		t.Error("Send after Close returned true")
	}
}

func TestDoneClosesOnClose(t *testing.T) {
	s := NewSession("s1", "/ws/test", nil)

	select {
	case <-s.Done():
		t.Fatal("Done closed before Close")
	default:
	}

	s.Close()

	select {
	case <-s.Done():
	default:
		t.Error("Done not closed after Close")
	}
}

func TestCloseIdempotent(t *testing.T) {
	s := NewSession("s1", "/ws/test", nil)
	s.Close()
	s.Close() // must not panic
}
