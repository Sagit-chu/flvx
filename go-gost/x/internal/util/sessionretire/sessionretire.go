package sessionretire

import "time"

const (
	defaultIdleGrace  = time.Second
	defaultPollPeriod = 100 * time.Millisecond
)

// Session is the lifecycle surface shared by the multiplexed dialers.
type Session interface {
	Close() error
	IsClosed() bool
	NumStreams() int
}

// Gracefully closes a retired session after all existing streams have drained.
// A short idle grace covers the Dial/Handshake hand-off used by several dialers.
func Gracefully(session Session) {
	if session == nil {
		return
	}
	go waitUntilIdle(session, defaultIdleGrace, defaultPollPeriod)
}

func waitUntilIdle(session Session, idleGrace, pollPeriod time.Duration) {
	if session == nil {
		return
	}
	if idleGrace <= 0 {
		idleGrace = defaultIdleGrace
	}
	if pollPeriod <= 0 {
		pollPeriod = defaultPollPeriod
	}

	ticker := time.NewTicker(pollPeriod)
	defer ticker.Stop()

	var idleSince time.Time
	for {
		if session.IsClosed() {
			_ = session.Close()
			return
		}
		if session.NumStreams() == 0 {
			if idleSince.IsZero() {
				idleSince = time.Now()
			} else if time.Since(idleSince) >= idleGrace {
				_ = session.Close()
				return
			}
		} else {
			idleSince = time.Time{}
		}
		<-ticker.C
	}
}
