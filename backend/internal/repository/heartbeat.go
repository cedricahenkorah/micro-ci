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

type Control struct {
	CancelRequested bool
	LeaseExpiresAt  time.Time
}

func (m RunModel) Heartbeat(
	ctx context.Context,
	claim Claim,
	leaseDuration time.Duration,
) (Control, error) {
	if claim.Run.ID == uuid.Nil {
		return Control{}, errors.New("runID must be a non-zero UUID")
	}

	if claim.AttemptID == uuid.Nil {
		return Control{}, errors.New("attemptID must be a non-zero UUID")
	}

	if strings.TrimSpace(claim.WorkerID) == "" {
		return Control{}, errors.New("workerID must not be empty")
	}

	if leaseDuration <= 0 {
		return Control{}, errors.New("leaseDuration must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	tx, txErr := m.DB.Begin(ctx)

	if txErr != nil {
		return Control{}, fmt.Errorf("begin heartbeat transaction: %w", txErr)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), m.queryTimeout,
		)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	var control Control

	runQuery := `
    SELECT cancel_requested_at IS NOT NULL
    FROM runs
    WHERE id = $1
      AND active_attempt_id = $2
      AND status = 'running'
    FOR UPDATE`

	err := tx.QueryRow(ctx, runQuery, claim.Run.ID, claim.AttemptID).
		Scan(&control.CancelRequested)

	if errors.Is(err, pgx.ErrNoRows) {
		return Control{}, domain.ErrLeaseLost
	}

	if err != nil {
		return Control{}, fmt.Errorf("lock heartbeat run: %w", err)
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

	err = tx.QueryRow(
		ctx, attemptQuery, claim.AttemptID, claim.Run.ID, claim.WorkerID,
	).Scan(&lockedAttemptID)

	if errors.Is(err, pgx.ErrNoRows) {
		return Control{}, domain.ErrLeaseLost
	}

	if err != nil {
		return Control{}, fmt.Errorf("lock heartbeat attempt: %w", err)
	}

	renewQuery := `
    UPDATE attempts
    SET lease_expires_at =
        clock_timestamp() + ($2::double precision * interval '1 second')
    WHERE id = $1
      AND lease_expires_at > clock_timestamp()
    RETURNING lease_expires_at`

	err = tx.QueryRow(
		ctx, renewQuery, lockedAttemptID, leaseDuration.Seconds(),
	).Scan(&control.LeaseExpiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return Control{}, domain.ErrLeaseLost
	}

	if err != nil {
		return Control{}, fmt.Errorf("renew heartbeat lease: %w", err)
	}

	err = tx.Commit(ctx)

	if err != nil {
		return Control{}, fmt.Errorf("commit heartbeat: %w", err)
	}

	return control, nil
}
