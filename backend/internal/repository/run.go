package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}

type CreateRunInput struct {
	Repository string
	Branch     string
	CommitSHA  string
	DeliveryID *string
}

func (m RunModel) CreateRun(
	ctx context.Context,
	input CreateRunInput,
) (domain.Run, error) {
	if strings.TrimSpace(input.Repository) == "" {
		return domain.Run{}, errors.New("repository is required")
	}

	if strings.TrimSpace(input.Branch) == "" {
		return domain.Run{}, errors.New("branch is required")
	}

	if strings.TrimSpace(input.CommitSHA) == "" {
		return domain.Run{}, errors.New("commit SHA is required")
	}

	if input.DeliveryID != nil && strings.TrimSpace(*input.DeliveryID) == "" {
		return domain.Run{}, errors.New("delivery ID must not be blank when provided")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	query := `INSERT INTO runs (repository, branch, commit_sha, webhook_delivery_id)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (webhook_delivery_id) DO NOTHING
	RETURNING id, repository, branch, commit_sha, status, active_attempt_id, cancel_requested_at, retry_of_run_id, created_at;`

	var r domain.Run

	err := m.DB.QueryRow(ctx, query, input.Repository, input.Branch, input.CommitSHA, input.DeliveryID).Scan(&r.ID, &r.Repository, &r.Branch, &r.CommitSHA, &r.Status, &r.ActiveAttemptID, &r.CancelRequestedAt, &r.RetryOfRunID, &r.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) && input.DeliveryID != nil {
		query := `
        SELECT id, repository, branch, commit_sha, status,
               active_attempt_id, cancel_requested_at, retry_of_run_id, created_at
        FROM runs
        WHERE webhook_delivery_id = $1`

		err = m.DB.QueryRow(ctx, query, *input.DeliveryID).Scan(
			&r.ID,
			&r.Repository,
			&r.Branch,
			&r.CommitSHA,
			&r.Status,
			&r.ActiveAttemptID,
			&r.CancelRequestedAt,
			&r.RetryOfRunID,
			&r.CreatedAt,
		)

		if err != nil {
			return domain.Run{}, fmt.Errorf("fetch existing run for delivery: %w", err)
		}

		return r, nil
	}

	if err != nil {
		return domain.Run{}, fmt.Errorf("create run: %w", err)
	}

	return r, nil
}
