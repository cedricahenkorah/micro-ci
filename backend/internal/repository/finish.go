package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type Outcome struct {
	Status      domain.AttemptStatus
	FailureKind domain.FailureKind
}

func (m RunModel) Finish(
	ctx context.Context,
	claim Claim,
	outcome Outcome,
) error {
	if claim.Run.ID <= 0 {
		return errors.New("runID must be positive")
	}

	if claim.AttemptID <= 0 {
		return errors.New("attemptID must be positive")
	}

	if strings.TrimSpace(claim.WorkerID) == "" {
		return errors.New("workerID must not be empty")
	}

	validateOutcomeErr := outcome.Validate()

	if validateOutcomeErr != nil {
		return validateOutcomeErr
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return fmt.Errorf("begin finish transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	runQuery := `
    SELECT cancel_requested_at IS NOT NULL
    FROM runs
    WHERE id = $1
      AND active_attempt_id = $2
      AND status = 'running'
    FOR UPDATE`

	var cancelRequested bool

	err := tx.QueryRow(ctx, runQuery, claim.Run.ID, claim.AttemptID).Scan(&cancelRequested)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf("lock finish run: %w", err)
	}

	if cancelRequested {
		outcome = Outcome{
			Status:      domain.AttemptStatusCancelled,
			FailureKind: domain.FailureKindCancelled,
		}
	}

	attemptQuery := `
    SELECT id, number
    FROM attempts
    WHERE id = $1
      AND run_id = $2
      AND worker_id = $3
      AND status = 'running'
    FOR UPDATE`

	var lockedAttemptID int64
	var attemptNumber int

	err = tx.QueryRow(ctx, attemptQuery, claim.AttemptID, claim.Run.ID, claim.WorkerID).Scan(&lockedAttemptID, &attemptNumber)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf("lock finish attempt: %w", err)
	}

	if outcome.Status == domain.AttemptStatusCancelled || outcome.Status == domain.AttemptStatusTimedOut {
		interruptStepsQuery := `
    UPDATE steps
    SET status = CASE
            WHEN status = 'running' THEN $2
            ELSE 'skipped'
        END,
        exit_code = NULL,
        finished_at = clock_timestamp()
    WHERE attempt_id = $1
      AND status IN ('running', 'pending')
    RETURNING step_index, status`

		rows, interruptStepsErr := tx.Query(ctx, interruptStepsQuery, claim.AttemptID, domain.StepStatus(outcome.Status))

		if interruptStepsErr != nil {
			return fmt.Errorf("finish interrupted attempt steps: %w", interruptStepsErr)
		}

		type interruptedStep struct {
			Index  int
			Status domain.StepStatus
		}

		steps, readErr := pgx.CollectRows(rows, pgx.RowToStructByPos[interruptedStep])

		if readErr != nil {
			return fmt.Errorf("read interrupted steps: %w", readErr)
		}

		sort.Slice(steps, func(i, j int) bool {
			return steps[i].Index < steps[j].Index
		})

		for _, step := range steps {
			payload, payloadErr := json.Marshal(struct {
				Status   domain.StepStatus `json:"status"`
				ExitCode *int              `json:"exit_code"`
			}{Status: step.Status})

			if payloadErr != nil {
				return fmt.Errorf("encode interrupted step %d event: %w", step.Index+1, payloadErr)
			}

			eventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
				AttemptID: &claim.AttemptID,
				Type:      domain.EventTypeStepStatus,
				StepIndex: &step.Index,
				Payload:   payload,
			})

			if eventErr != nil {
				return fmt.Errorf("append interrupted step %d event: %w", step.Index+1, eventErr)
			}
		}
	}

	finishAttemptQuery := `
    UPDATE attempts
    SET status = $2,
        failure_kind = NULLIF($3::text, ''),
        finished_at = clock_timestamp()
    WHERE id = $1
      AND lease_expires_at > clock_timestamp()
    RETURNING id`

	err = tx.QueryRow(ctx, finishAttemptQuery, lockedAttemptID, outcome.Status, outcome.FailureKind).Scan(&lockedAttemptID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf("finish attempt: %w", err)
	}

	runStatus := domain.RunStatus(outcome.Status)

	if outcome.Status == domain.AttemptStatusFailed && outcome.FailureKind == domain.FailureKindInfrastructure && attemptNumber < 2 {
		runStatus = domain.RunStatusQueued
	}

	finishRunQuery := `
    UPDATE runs
    SET status = $2, active_attempt_id = NULL
    WHERE id = $1
      AND active_attempt_id = $3
      AND status = 'running'
    RETURNING id`

	var finishedRunID int64

	err = tx.QueryRow(ctx, finishRunQuery, claim.Run.ID, runStatus, claim.AttemptID).Scan(&finishedRunID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}

	attemptPayload, attemptPayloadErr := json.Marshal(struct {
		Status      domain.AttemptStatus `json:"status"`
		FailureKind domain.FailureKind   `json:"failure_kind,omitempty"`
	}{Status: outcome.Status, FailureKind: outcome.FailureKind})

	if attemptPayloadErr != nil {
		return fmt.Errorf("encode finished attempt event: %w", attemptPayloadErr)
	}

	attemptEventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
		AttemptID: &claim.AttemptID,
		Type:      domain.EventTypeAttemptStatus,
		Payload:   attemptPayload,
	})

	if attemptEventErr != nil {
		return fmt.Errorf("append finished attempt event: %w", attemptEventErr)
	}

	runPayload, runPayloadErr := json.Marshal(struct {
		Status domain.RunStatus `json:"status"`
	}{Status: runStatus})

	if runPayloadErr != nil {
		return fmt.Errorf("encode finished run event: %w", runPayloadErr)
	}

	runEventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
		AttemptID: &claim.AttemptID,
		Type:      domain.EventTypeRunStatus,
		Payload:   runPayload,
	})

	if runEventErr != nil {
		return fmt.Errorf("append finished run event: %w", runEventErr)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finish: %w", err)
	}

	return nil
}

func (o Outcome) Validate() error {
	switch o.Status {
	case domain.AttemptStatusSucceeded:
		if o.FailureKind == "" {
			return nil
		}

	case domain.AttemptStatusFailed:
		switch o.FailureKind {
		case domain.FailureKindConfig,
			domain.FailureKindCheckout,
			domain.FailureKindCommand,
			domain.FailureKindInfrastructure:
			return nil
		}

	case domain.AttemptStatusTimedOut:
		if o.FailureKind == domain.FailureKindTimeout {
			return nil
		}

	case domain.AttemptStatusCancelled:
		if o.FailureKind == domain.FailureKindCancelled {
			return nil
		}

	default:
		return fmt.Errorf("invalid finish status %q", o.Status)
	}

	return fmt.Errorf(
		"failure kind %q is not valid for status %q",
		o.FailureKind, o.Status,
	)
}
