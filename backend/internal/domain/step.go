package domain

import "time"

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
	StepStatusCancelled StepStatus = "cancelled"
	StepStatusTimedOut  StepStatus = "timed_out"
	StepStatusLost      StepStatus = "lost"
)

// Step records a command's execution within an attempt.
type Step struct {
	AttemptID  int64      `json:"attempt_id"`
	Index      int        `json:"index"`
	Name       string     `json:"name"`
	Command    string     `json:"command"`
	Status     StepStatus `json:"status"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
}
