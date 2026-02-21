package audit

import (
	"encoding/json"
	"log"
	"time"
)

type ToolEvent struct {
	TS         string `json:"ts"`
	Tool       string `json:"tool"`
	Outcome    string `json:"outcome"`
	DurationMS int64  `json:"duration_ms"`
	Message    string `json:"message,omitempty"`
}

type Logger struct {
	enabled bool
}

func NewLogger(enabled bool) *Logger {
	return &Logger{enabled: enabled}
}

func (l *Logger) LogToolCall(tool string, startedAt time.Time, err error) {
	if l == nil || !l.enabled {
		return
	}

	event := ToolEvent{
		TS:         time.Now().UTC().Format(time.RFC3339Nano),
		Tool:       tool,
		Outcome:    "ok",
		DurationMS: time.Since(startedAt).Milliseconds(),
	}
	if err != nil {
		event.Outcome = "error"
		event.Message = err.Error()
	}

	b, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		log.Printf("audit marshal error: %v", marshalErr)
		return
	}
	log.Printf("mcp_audit %s", string(b))
}
