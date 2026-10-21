package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type expiredRun struct {
	RunID           int64
	AttemptID       int64
	AttemptNumber   int
	CancelRequested bool
}

func (m RunModel) RecoverExpired(
	ctx context.Context,
	maxAttempts int,
) error {
	if maxAttempts <= 0 {
		return errors.New("maxAttempts must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return fmt.Errorf("begin recovery transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)

		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	expiredQuery := `
SELECT r.id, a.id, a.number,
       r.cancel_requested_at IS NOT NULL
FROM runs AS r
JOIN attempts AS a
  ON a.id = r.active_attempt_id
 AND a.run_id = r.id
WHERE r.status = 'running'
  AND a.status = 'running'
  AND a.lease_expires_at <= clock_timestamp()
ORDER BY r.id
FOR UPDATE OF r SKIP LOCKED`

	rows, queryErr := tx.Query(ctx, expiredQuery)

	if queryErr != nil {
		return fmt.Errorf("select expired runs: %w", queryErr)
	}

	defer rows.Close()

	var expiredRuns []expiredRun

	for rows.Next() {
		var run expiredRun

		scanErr := rows.Scan(
			&run.RunID,
			&run.AttemptID,
			&run.AttemptNumber,
			&run.CancelRequested,
		)

		if scanErr != nil {
			return fmt.Errorf("scan expired run: %w", scanErr)
		}

		expiredRuns = append(expiredRuns, run)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("read expired runs: %w", err)
	}

	rows.Close()

	for _, run := range expiredRuns {
		attemptQuery := `
UPDATE attempts
SET status = 'lost',
    failure_kind = 'infrastructure',
    finished_at = clock_timestamp()
WHERE id = $1
  AND run_id = $2
  AND status = 'running'
  AND lease_expires_at <= clock_timestamp()`

		attemptTag, attemptErr := tx.Exec(ctx, attemptQuery, run.AttemptID, run.RunID)

		if attemptErr != nil {
			return fmt.Errorf("mark expired attempt lost: %w", attemptErr)
		}

		if attemptTag.RowsAffected() == 0 {
			continue
		}

		stepsQuery := `
UPDATE steps
SET status = CASE
        WHEN status = 'running' THEN 'lost'
        ELSE 'skipped'
    END,
    exit_code = NULL,
    finished_at = clock_timestamp()
WHERE attempt_id = $1
  AND status IN ('running', 'pending')
RETURNING step_index, status`

		rows, stepsErr := tx.Query(ctx, stepsQuery, run.AttemptID)

		if stepsErr != nil {
			return fmt.Errorf("finish expired attempt steps: %w", stepsErr)
		}

		type recoveredStep struct {
			Index  int
			Status domain.StepStatus
		}

		steps, readErr := pgx.CollectRows(rows, pgx.RowToStructByPos[recoveredStep])

		if readErr != nil {
			return fmt.Errorf("read recovered steps: %w", readErr)
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
				return fmt.Errorf("encode recovered step %d event: %w", step.Index+1, payloadErr)
			}

			eventErr := AppendEvent(ctx, tx, run.RunID, CreateEventInput{
				AttemptID: &run.AttemptID,
				Type:      domain.EventTypeStepStatus,
				StepIndex: &step.Index,
				Payload:   payload,
			})

			if eventErr != nil {
				return fmt.Errorf("append recovered step %d event: %w", step.Index+1, eventErr)
			}
		}

		runStatus := domain.RunStatusFailed

		if run.CancelRequested {
			runStatus = domain.RunStatusCancelled
		} else if run.AttemptNumber < maxAttempts {
			runStatus = domain.RunStatusQueued
		}

		runQuery := `
UPDATE runs
SET status = $2, active_attempt_id = NULL
WHERE id = $1
  AND active_attempt_id = $3
  AND status = 'running'`

		runTag, runErr := tx.Exec(ctx, runQuery, run.RunID, runStatus, run.AttemptID)

		if runErr != nil {
			return fmt.Errorf("recover run: %w", runErr)
		}

		if runTag.RowsAffected() != 1 {
			return fmt.Errorf("recover run %d: %w", run.RunID, domain.ErrInvalidState)
		}

		attemptEventErr := AppendEvent(ctx, tx, run.RunID, CreateEventInput{
			AttemptID: &run.AttemptID,
			Type:      domain.EventTypeAttemptStatus,
			Payload:   []byte(`{"status":"lost","failure_kind":"infrastructure"}`),
		})

		if attemptEventErr != nil {
			return fmt.Errorf("append lost attempt event: %w", attemptEventErr)
		}

		runPayload, runPayloadErr := json.Marshal(struct {
			Status domain.RunStatus `json:"status"`
		}{Status: runStatus})

		if runPayloadErr != nil {
			return fmt.Errorf("encode recovered run event: %w", runPayloadErr)
		}

		runEventErr := AppendEvent(ctx, tx, run.RunID, CreateEventInput{
			AttemptID: &run.AttemptID,
			Type:      domain.EventTypeRunStatus,
			Payload:   runPayload,
		})

		if runEventErr != nil {
			return fmt.Errorf("append recovered run event: %w", runEventErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recovery: %w", err)
	}

	return nil
}
