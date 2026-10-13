package domain

import "time"

type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusTimedOut  RunStatus = "timed_out"
	RunStatusCancelled RunStatus = "cancelled"
)

// Run is a request to execute a pipeline at a specific commit.
type Run struct {
	ID                int64      `json:"id"`
	Repository        string     `json:"repository"`
	Branch            string     `json:"branch"`
	CommitSHA         string     `json:"commit_sha"`
	Status            RunStatus  `json:"status"`
	ActiveAttemptID   *int64     `json:"active_attempt_id,omitempty"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}
