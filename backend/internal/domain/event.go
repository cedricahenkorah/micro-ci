package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventTypeRunStatus     EventType = "run.status"
	EventTypeAttemptStatus EventType = "attempt.status"
	EventTypeStepStatus    EventType = "step.status"
	EventTypeLog           EventType = "log"
	EventTypeLogTruncated  EventType = "log.truncated"
)

// Event is a persisted lifecycle update or log chunk, ordered within a run.
type Event struct {
	RunID     uuid.UUID       `json:"run_id"`
	AttemptID *uuid.UUID      `json:"attempt_id,omitempty"`
	Sequence  int64           `json:"sequence"`
	Type      EventType       `json:"type"`
	StepIndex *int            `json:"step_index,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}
