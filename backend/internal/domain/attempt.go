package domain

import "time"

type AttemptStatus string

const (
	AttemptStatusRunning   AttemptStatus = "running"
	AttemptStatusSucceeded AttemptStatus = "succeeded"
	AttemptStatusFailed    AttemptStatus = "failed"
	AttemptStatusTimedOut  AttemptStatus = "timed_out"
	AttemptStatusCancelled AttemptStatus = "cancelled"
	AttemptStatusLost      AttemptStatus = "lost"
)

type FailureKind string

const (
	FailureKindConfig         FailureKind = "config"
	FailureKindCheckout       FailureKind = "checkout"
	FailureKindCommand        FailureKind = "command"
	FailureKindInfrastructure FailureKind = "infrastructure"
	FailureKindTimeout        FailureKind = "timeout"
	FailureKindCancelled      FailureKind = "cancelled"
)

// Attempt records one execution of a run by a worker holding a lease.
type Attempt struct {
	ID             int64         `json:"id"`
	RunID          int64         `json:"run_id"`
	Number         int           `json:"number"`
	WorkerID       string        `json:"worker_id"`
	LeaseExpiresAt time.Time     `json:"lease_expires_at"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     *time.Time    `json:"finished_at,omitempty"`
	Status         AttemptStatus `json:"status"`
	FailureKind    FailureKind   `json:"failure_kind,omitempty"`
}
