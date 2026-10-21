package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (m RunModel) RetryRun(
	ctx context.Context,
	runID int64,
) (domain.Run, error) {
	if runID <= 0 {
		return domain.Run{}, errors.New("runID must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	tx, err := m.DB.Begin(ctx)

	if err != nil {
		return domain.Run{}, fmt.Errorf("begin retry transaction: %w", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)

		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	lockQuery := `
	SELECT repository, branch, commit_sha, status
	FROM runs
	WHERE id = $1
	FOR UPDATE`

	var source domain.Run

	err = tx.QueryRow(ctx, lockQuery, runID).Scan(
		&source.Repository,
		&source.Branch,
		&source.CommitSHA,
		&source.Status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Run{}, domain.ErrNotFound
	}

	if err != nil {
		return domain.Run{}, fmt.Errorf("lock retry run: %w", err)
	}

	if source.Status == domain.RunStatusQueued || source.Status == domain.RunStatusRunning {
		return domain.Run{}, domain.ErrInvalidState
	}

	insertQuery := `
	INSERT INTO runs (repository, branch, commit_sha, retry_of_run_id)
	VALUES ($1, $2, $3, $4)
	RETURNING id, repository, branch, commit_sha, status,
	          active_attempt_id, cancel_requested_at, retry_of_run_id, created_at`

	var run domain.Run

	err = tx.QueryRow(
		ctx, insertQuery, source.Repository, source.Branch, source.CommitSHA, runID,
	).Scan(
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

	if err != nil {
		return domain.Run{}, fmt.Errorf("create retry run: %w", err)
	}

	err = AppendEvent(ctx, tx, run.ID, CreateEventInput{
		Type:    domain.EventTypeRunStatus,
		Payload: []byte(`{"status":"queued"}`),
	})

	if err != nil {
		return domain.Run{}, fmt.Errorf("append retry run event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Run{}, fmt.Errorf("commit retry: %w", err)
	}

	return run, nil
}
