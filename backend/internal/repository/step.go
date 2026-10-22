package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StepModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}

type CreateStepInput struct {
	Name    string
	Command string
}

type StepResult struct {
	Status   domain.StepStatus
	ExitCode *int
}

func (m StepModel) CreateSteps(
	ctx context.Context,
	claim Claim,
	steps []CreateStepInput,
) error {
	if claim.Run.ID == uuid.Nil {
		return errors.New("runID must be a non-zero UUID")
	}

	if claim.AttemptID == uuid.Nil {
		return errors.New("attemptID must be a non-zero UUID")
	}

	if strings.TrimSpace(claim.WorkerID) == "" {
		return errors.New("workerID must not be empty")
	}

	if len(steps) == 0 {
		return errors.New("steps must not be empty")
	}

	for i, step := range steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("step %d: name is required", i+1)
		}

		if strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("step %d: command is required", i+1)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return fmt.Errorf("begin create steps transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	runQuery := `SELECT id
	FROM runs
	WHERE id = $1
  	AND active_attempt_id = $2
   	AND status = 'running'
	FOR UPDATE`

	var lockedRunID uuid.UUID

	runQueryErr := tx.QueryRow(ctx, runQuery, claim.Run.ID, claim.AttemptID).Scan(&lockedRunID)

	if errors.Is(runQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if runQueryErr != nil {
		return fmt.Errorf("lock create steps run: %w", runQueryErr)
	}

	attemptQuery := `SELECT id
	FROM attempts
	WHERE id = $1
  	AND run_id = $2
  	AND worker_id = $3
  	AND status = 'running'
	FOR UPDATE`

	var lockedAttemptID uuid.UUID

	attemptQueryErr := tx.QueryRow(ctx, attemptQuery, claim.AttemptID, claim.Run.ID, claim.WorkerID).Scan(&lockedAttemptID)

	if errors.Is(attemptQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if attemptQueryErr != nil {
		return fmt.Errorf("lock create steps attempt: %w", attemptQueryErr)
	}

	insertQuery := `
    INSERT INTO steps (attempt_id, step_index, name, command, status)
    SELECT id, $2, $3, $4, 'pending'
    FROM attempts
    WHERE id = $1
    AND lease_expires_at > clock_timestamp()`

	for i, step := range steps {
		tag, insertErr := tx.Exec(
			ctx,
			insertQuery,
			claim.AttemptID,
			i,
			step.Name,
			step.Command,
		)

		if insertErr != nil {
			return fmt.Errorf("insert step %d: %w", i+1, insertErr)
		}

		if tag.RowsAffected() != 1 {
			return domain.ErrLeaseLost
		}

		eventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
			AttemptID: &claim.AttemptID,
			Type:      domain.EventTypeStepStatus,
			StepIndex: &i,
			Payload:   []byte(`{"status":"pending"}`),
		})

		if eventErr != nil {
			return fmt.Errorf("append created step %d event: %w", i+1, eventErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create steps: %w", err)
	}

	return nil
}

func (m StepModel) StartStep(
	ctx context.Context,
	claim Claim,
	index int,
) error {
	if claim.Run.ID == uuid.Nil {
		return errors.New("runID must be a non-zero UUID")
	}

	if claim.AttemptID == uuid.Nil {
		return errors.New("attemptID must be a non-zero UUID")
	}

	if strings.TrimSpace(claim.WorkerID) == "" {
		return errors.New("workerID must not be empty")
	}

	if index < 0 {
		return errors.New("step index must not be negative")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return fmt.Errorf("begin start step transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	runQuery := `SELECT id
	FROM runs
	WHERE id = $1
  	AND active_attempt_id = $2
   	AND status = 'running'
	FOR UPDATE`

	var lockedRunID uuid.UUID

	runQueryErr := tx.QueryRow(ctx, runQuery, claim.Run.ID, claim.AttemptID).Scan(&lockedRunID)

	if errors.Is(runQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if runQueryErr != nil {
		return fmt.Errorf("lock start step run: %w", runQueryErr)
	}

	attemptQuery := `SELECT id
	FROM attempts
	WHERE id = $1
  	AND run_id = $2
  	AND worker_id = $3
  	AND status = 'running'
	FOR UPDATE`

	var lockedAttemptID uuid.UUID

	attemptQueryErr := tx.QueryRow(ctx, attemptQuery, claim.AttemptID, claim.Run.ID, claim.WorkerID).Scan(&lockedAttemptID)

	if errors.Is(attemptQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if attemptQueryErr != nil {
		return fmt.Errorf("lock start step attempt: %w", attemptQueryErr)
	}

	stepQuery := `
    SELECT status
    FROM steps
    WHERE attempt_id = $1
      AND step_index = $2
    FOR UPDATE`

	var status domain.StepStatus

	stepErr := tx.QueryRow(
		ctx, stepQuery, claim.AttemptID, index,
	).Scan(&status)

	if errors.Is(stepErr, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}

	if stepErr != nil {
		return fmt.Errorf("lock start step: %w", stepErr)
	}

	if status != domain.StepStatusPending {
		return domain.ErrInvalidState
	}

	startQuery := `
    UPDATE steps AS s
    SET status = 'running',
        started_at = clock_timestamp()
    FROM attempts AS a
    WHERE s.attempt_id = $1
      AND s.step_index = $2
      AND s.status = 'pending'
      AND a.id = s.attempt_id
      AND a.lease_expires_at > clock_timestamp()`

	tag, startErr := tx.Exec(ctx, startQuery, claim.AttemptID, index)

	if startErr != nil {
		return fmt.Errorf("start step %d: %w", index+1, startErr)
	}

	if tag.RowsAffected() != 1 {
		return domain.ErrLeaseLost
	}

	eventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
		AttemptID: &claim.AttemptID,
		Type:      domain.EventTypeStepStatus,
		StepIndex: &index,
		Payload:   []byte(`{"status":"running"}`),
	})

	if eventErr != nil {
		return fmt.Errorf("append started step %d event: %w", index+1, eventErr)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit start step: %w", err)
	}

	return nil
}

func (m StepModel) FinishStep(
	ctx context.Context,
	claim Claim,
	index int,
	result StepResult,
) error {
	if claim.Run.ID == uuid.Nil {
		return errors.New("runID must be a non-zero UUID")
	}

	if claim.AttemptID == uuid.Nil {
		return errors.New("attemptID must be a non-zero UUID")
	}

	if strings.TrimSpace(claim.WorkerID) == "" {
		return errors.New("workerID must not be empty")
	}

	if index < 0 {
		return errors.New("step index must not be negative")
	}

	validationErr := result.Validate()

	if validationErr != nil {
		return validationErr
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return fmt.Errorf("begin finish step transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)

		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	runQuery := `SELECT id
	FROM runs
	WHERE id = $1
  	AND active_attempt_id = $2
   	AND status = 'running'
	FOR UPDATE`

	var lockedRunID uuid.UUID

	runQueryErr := tx.QueryRow(ctx, runQuery, claim.Run.ID, claim.AttemptID).Scan(&lockedRunID)

	if errors.Is(runQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if runQueryErr != nil {
		return fmt.Errorf("lock finish step run: %w", runQueryErr)
	}

	attemptQuery := `SELECT id
	FROM attempts
	WHERE id = $1
  	AND run_id = $2
  	AND worker_id = $3
  	AND status = 'running'
	FOR UPDATE`

	var lockedAttemptID uuid.UUID

	attemptQueryErr := tx.QueryRow(ctx, attemptQuery, claim.AttemptID, claim.Run.ID, claim.WorkerID).Scan(&lockedAttemptID)

	if errors.Is(attemptQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if attemptQueryErr != nil {
		return fmt.Errorf("lock finish step attempt: %w", attemptQueryErr)
	}

	stepQuery := `
    SELECT status
    FROM steps
    WHERE attempt_id = $1
      AND step_index = $2
    FOR UPDATE`

	var status domain.StepStatus

	stepErr := tx.QueryRow(
		ctx, stepQuery, claim.AttemptID, index,
	).Scan(&status)

	if errors.Is(stepErr, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}

	if stepErr != nil {
		return fmt.Errorf("lock finish step: %w", stepErr)
	}

	if status != domain.StepStatusRunning {
		return domain.ErrInvalidState
	}

	finishQuery := `
    UPDATE steps AS s
    SET status = $3,
        exit_code = $4,
        finished_at = clock_timestamp()
    FROM attempts AS a
    WHERE s.attempt_id = $1
      AND s.step_index = $2
      AND s.status = 'running'
      AND a.id = s.attempt_id
      AND a.lease_expires_at > clock_timestamp()`

	tag, finishErr := tx.Exec(
		ctx,
		finishQuery,
		claim.AttemptID,
		index,
		result.Status,
		result.ExitCode,
	)

	if finishErr != nil {
		return fmt.Errorf("finish step %d: %w", index+1, finishErr)
	}

	if tag.RowsAffected() != 1 {
		return domain.ErrLeaseLost
	}

	payload, payloadErr := json.Marshal(struct {
		Status   domain.StepStatus `json:"status"`
		ExitCode *int              `json:"exit_code"`
	}{Status: result.Status, ExitCode: result.ExitCode})

	if payloadErr != nil {
		return fmt.Errorf("encode finished step %d event: %w", index+1, payloadErr)
	}

	eventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
		AttemptID: &claim.AttemptID,
		Type:      domain.EventTypeStepStatus,
		StepIndex: &index,
		Payload:   payload,
	})

	if eventErr != nil {
		return fmt.Errorf("append finished step %d event: %w", index+1, eventErr)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finish step: %w", err)
	}

	return nil
}

func (m StepModel) SkipPendingSteps(
	ctx context.Context,
	claim Claim,
) error {
	if claim.Run.ID == uuid.Nil {
		return errors.New("runID must be a non-zero UUID")
	}

	if claim.AttemptID == uuid.Nil {
		return errors.New("attemptID must be a non-zero UUID")
	}

	if strings.TrimSpace(claim.WorkerID) == "" {
		return errors.New("workerID must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return fmt.Errorf("begin skip pending steps transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	runQuery := `
    SELECT id
    FROM runs
    WHERE id = $1
      AND active_attempt_id = $2
      AND status = 'running'
    FOR UPDATE`

	var lockedRunID uuid.UUID

	runQueryErr := tx.QueryRow(ctx, runQuery, claim.Run.ID, claim.AttemptID).Scan(&lockedRunID)

	if errors.Is(runQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if runQueryErr != nil {
		return fmt.Errorf("lock skip pending steps run: %w", runQueryErr)
	}

	attemptQuery := `
    SELECT id
    FROM attempts
    WHERE id = $1
      AND run_id = $2
      AND worker_id = $3
      AND status = 'running'
    FOR UPDATE`

	var lockedAttemptID uuid.UUID

	attemptQueryErr := tx.QueryRow(ctx, attemptQuery, claim.AttemptID, claim.Run.ID, claim.WorkerID).Scan(&lockedAttemptID)

	if errors.Is(attemptQueryErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if attemptQueryErr != nil {
		return fmt.Errorf("lock skip pending steps attempt: %w", attemptQueryErr)
	}

	skipQuery := `
    UPDATE steps
    SET status = 'skipped',
        finished_at = clock_timestamp()
    WHERE attempt_id = $1
      AND status = 'pending'
    RETURNING step_index`

	rows, skipErr := tx.Query(ctx, skipQuery, claim.AttemptID)

	if skipErr != nil {
		return fmt.Errorf("skip pending steps: %w", skipErr)
	}

	indexes, readErr := pgx.CollectRows(rows, pgx.RowTo[int])

	if readErr != nil {
		return fmt.Errorf("read skipped step indexes: %w", readErr)
	}

	sort.Ints(indexes)

	for _, index := range indexes {
		eventErr := AppendEvent(ctx, tx, claim.Run.ID, CreateEventInput{
			AttemptID: &claim.AttemptID,
			Type:      domain.EventTypeStepStatus,
			StepIndex: &index,
			Payload:   []byte(`{"status":"skipped"}`),
		})

		if eventErr != nil {
			return fmt.Errorf("append skipped step %d event: %w", index+1, eventErr)
		}
	}

	leaseQuery := `
    SELECT id
    FROM attempts
    WHERE id = $1
      AND lease_expires_at > clock_timestamp()`

	var validAttemptID uuid.UUID

	leaseErr := tx.QueryRow(ctx, leaseQuery, claim.AttemptID).Scan(&validAttemptID)

	if errors.Is(leaseErr, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if leaseErr != nil {
		return fmt.Errorf("check skip pending steps lease: %w", leaseErr)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit skip pending steps: %w", err)
	}

	return nil
}

func (r StepResult) Validate() error {
	switch r.Status {
	case domain.StepStatusSucceeded:
		if r.ExitCode == nil || *r.ExitCode != 0 {
			return errors.New("succeeded step requires exit code 0")
		}

	case domain.StepStatusFailed:
		if r.ExitCode != nil && *r.ExitCode <= 0 {
			return errors.New("failed step requires a positive exit code or nil")
		}

	default:
		return fmt.Errorf("invalid finish step status %q", r.Status)
	}

	return nil
}
