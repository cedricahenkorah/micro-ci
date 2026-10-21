package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (m RunModel) RequestCancel(
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
		return domain.Run{}, fmt.Errorf("begin cancel transaction: %w", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)

		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	lockQuery := `
	SELECT id, repository, branch, commit_sha, status,
	       active_attempt_id, cancel_requested_at, retry_of_run_id, created_at
	FROM runs
	WHERE id = $1
	FOR UPDATE`

	var run domain.Run

	err = tx.QueryRow(ctx, lockQuery, runID).Scan(
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

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Run{}, domain.ErrNotFound
	}

	if err != nil {
		return domain.Run{}, fmt.Errorf("lock cancel run: %w", err)
	}

	if run.Status == domain.RunStatusQueued || run.Status == domain.RunStatusRunning {
		cancelQuery := `
		UPDATE runs
		SET cancel_requested_at = COALESCE(
		        cancel_requested_at, clock_timestamp()
		    ),
		    status = CASE
		        WHEN status = 'queued' THEN 'cancelled'
		        ELSE status
		    END
		WHERE id = $1
		RETURNING status, cancel_requested_at`

		err = tx.QueryRow(ctx, cancelQuery, runID).Scan(
			&run.Status,
			&run.CancelRequestedAt,
		)

		if err != nil {
			return domain.Run{}, fmt.Errorf("request run cancellation: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Run{}, fmt.Errorf("commit cancel: %w", err)
	}

	return run, nil
}
