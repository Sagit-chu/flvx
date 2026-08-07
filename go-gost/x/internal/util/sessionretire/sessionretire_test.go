package sessionretire

import (
	"sync"
	"testing"
	"time"
)

type testSession struct {
	mu      sync.Mutex
	streams int
	closed  bool
}

func (s *testSession) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func (s *testSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *testSession) NumStreams() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams
}

func TestWaitUntilIdlePreservesActiveStreams(t *testing.T) {
	session := &testSession{streams: 1}
	done := make(chan struct{})
	go func() {
		waitUntilIdle(session, 20*time.Millisecond, time.Millisecond)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	if session.IsClosed() {
		t.Fatal("active session was closed")
	}

	session.mu.Lock()
	session.streams = 0
	session.mu.Unlock()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("idle session was not closed")
	}
	if !session.IsClosed() {
		t.Fatal("retired session did not close after becoming idle")
	}
}
