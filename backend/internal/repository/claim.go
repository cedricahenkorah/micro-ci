package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Claim struct {
	Run            domain.Run
	AttemptID      uuid.UUID
	WorkerID       string
	StartedAt      time.Time
	LeaseExpiresAt time.Time
}

func (m RunModel) Claim(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (*Claim, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, errors.New("workerID is required")
	}

	if leaseDuration <= 0 {
		return nil, errors.New("leaseDuration must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return nil, fmt.Errorf("begin claim transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	query := `SELECT id, repository, branch, commit_sha, status,
           active_attempt_id, cancel_requested_at, retry_of_run_id, created_at
    FROM runs
    WHERE status = 'queued'
    ORDER BY created_at, id
    LIMIT 1
    FOR UPDATE SKIP LOCKED`

	var run domain.Run

	txQueryErr := tx.QueryRow(ctx, query).Scan(
		&run.ID,
		&run.Repository,
		&run.Branch,
		&run.CommitSHA,
		&run.Status,
		&run.ActiveAttemptID,
		&run.CancelRequestedAt,
		&run.RetryOfRunID,
		&run.CreatedAt,
	)

	if errors.Is(txQueryErr, pgx.ErrNoRows) {
		return nil, nil
	}

	if txQueryErr != nil {
		return nil, fmt.Errorf("lock queued run: %w", txQueryErr)
	}

	insertQuery := `
    INSERT INTO attempts (run_id, number, worker_id, started_at, lease_expires_at)
    VALUES (
        $1,
        (SELECT COALESCE(MAX(number), 0) + 1
         FROM attempts WHERE run_id = $1),
        $2,
        clock_timestamp(),
        clock_timestamp() + ($3::double precision * interval '1 second')
    )
    RETURNING id, started_at, lease_expires_at`

	claim := Claim{Run: run, WorkerID: workerID}

	insertErr := tx.QueryRow(
		ctx, insertQuery, run.ID, workerID, leaseDuration.Seconds(),
	).Scan(&claim.AttemptID, &claim.StartedAt, &claim.LeaseExpiresAt)

	if insertErr != nil {
		return nil, fmt.Errorf("insert claim attempt: %w", insertErr)
	}

	updateQuery := `
    UPDATE runs
    SET status = 'running', active_attempt_id = $2
    WHERE id = $1
    RETURNING status, active_attempt_id`

	updateErr := tx.QueryRow(ctx, updateQuery, run.ID, claim.AttemptID).Scan(
		&claim.Run.Status,
		&claim.Run.ActiveAttemptID,
	)

	if updateErr != nil {
		return nil, fmt.Errorf("update claimed run: %w", updateErr)
	}

	attemptEventErr := AppendEvent(ctx, tx, run.ID, CreateEventInput{
		AttemptID: &claim.AttemptID,
		Type:      domain.EventTypeAttemptStatus,
		Payload:   []byte(`{"status":"running"}`),
	})

	if attemptEventErr != nil {
		return nil, fmt.Errorf("append claimed attempt event: %w", attemptEventErr)
	}

	runEventErr := AppendEvent(ctx, tx, run.ID, CreateEventInput{
		AttemptID: &claim.AttemptID,
		Type:      domain.EventTypeRunStatus,
		Payload:   []byte(`{"status":"running"}`),
	})

	if runEventErr != nil {
		return nil, fmt.Errorf("append claimed run event: %w", runEventErr)
	}

	commitErr := tx.Commit(ctx)

	if commitErr != nil {
		return nil, fmt.Errorf("commit claim: %w", commitErr)
	}

	return &claim, nil
}
