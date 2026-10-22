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

type PaginatedResult[T any] struct {
	Data         []T   `json:"data"`
	TotalCount   int64 `json:"totalCount"`
	TotalPages   int64 `json:"totalPages"`
	CurrentPage  int64 `json:"currentPage"`
	HasMore      bool  `json:"hasMore"`
	ItemsPerPage int   `json:"itemsPerPage"`
}

type RunSummary struct {
	domain.Run
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type RunDetails struct {
	domain.Run
	Attempts []AttemptDetails `json:"attempts"`
}

type AttemptDetails struct {
	domain.Attempt
	Steps []domain.Step `json:"steps"`
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

	tx, err := m.DB.Begin(ctx)

	if err != nil {
		return domain.Run{}, fmt.Errorf("begin create run transaction: %w", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), m.queryTimeout)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	query := `INSERT INTO runs (repository, branch, commit_sha, webhook_delivery_id)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (webhook_delivery_id) DO NOTHING
	RETURNING id, repository, branch, commit_sha, status, active_attempt_id, cancel_requested_at, retry_of_run_id, created_at;`

	var r domain.Run

	err = tx.QueryRow(ctx, query, input.Repository, input.Branch, input.CommitSHA, input.DeliveryID).Scan(&r.ID, &r.Repository, &r.Branch, &r.CommitSHA, &r.Status, &r.ActiveAttemptID, &r.CancelRequestedAt, &r.RetryOfRunID, &r.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) && input.DeliveryID != nil {
		query := `
        SELECT id, repository, branch, commit_sha, status,
               active_attempt_id, cancel_requested_at, retry_of_run_id, created_at
        FROM runs
        WHERE webhook_delivery_id = $1`

		err = tx.QueryRow(ctx, query, *input.DeliveryID).Scan(
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

	err = AppendEvent(ctx, tx, r.ID, CreateEventInput{
		Type:    domain.EventTypeRunStatus,
		Payload: []byte(`{"status":"queued"}`),
	})

	if err != nil {
		return domain.Run{}, fmt.Errorf("append created run event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Run{}, fmt.Errorf("commit create run: %w", err)
	}

	return r, nil
}

func (m RunModel) ListRuns(ctx context.Context, limit int, page int64) (PaginatedResult[RunSummary], error) {
	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})

	if err != nil {
		return PaginatedResult[RunSummary]{}, fmt.Errorf("begin run listing transaction: %w", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), m.queryTimeout)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	result := PaginatedResult[RunSummary]{
		Data:         make([]RunSummary, 0),
		CurrentPage:  page,
		ItemsPerPage: limit,
	}

	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM runs`).Scan(&result.TotalCount); err != nil {
		return PaginatedResult[RunSummary]{}, fmt.Errorf("count runs: %w", err)
	}

	result.TotalPages = (result.TotalCount + int64(limit) - 1) / int64(limit)
	result.HasMore = page+1 < result.TotalPages

	query := `
SELECT r.id, r.repository, r.branch, r.commit_sha, r.status,
       r.active_attempt_id, r.cancel_requested_at, r.retry_of_run_id, r.created_at,
       timing.started_at,
       CASE WHEN r.status NOT IN ('queued', 'running') THEN timing.finished_at END
FROM (SELECT * FROM runs ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2) r
LEFT JOIN LATERAL (
    SELECT MIN(started_at) AS started_at, MAX(finished_at) AS finished_at
    FROM attempts WHERE run_id = r.id
) timing ON true
ORDER BY r.created_at DESC, r.id DESC`

	rows, err := tx.Query(ctx, query, limit, page*int64(limit))

	if err != nil {
		return PaginatedResult[RunSummary]{}, fmt.Errorf("list runs: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var run RunSummary

		if err := rows.Scan(
			&run.ID, &run.Repository, &run.Branch, &run.CommitSHA, &run.Status,
			&run.ActiveAttemptID, &run.CancelRequestedAt, &run.RetryOfRunID, &run.CreatedAt,
			&run.StartedAt, &run.FinishedAt,
		); err != nil {
			return PaginatedResult[RunSummary]{}, fmt.Errorf("scan run: %w", err)
		}

		result.Data = append(result.Data, run)
	}

	if err := rows.Err(); err != nil {
		return PaginatedResult[RunSummary]{}, fmt.Errorf("read runs: %w", err)
	}

	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return PaginatedResult[RunSummary]{}, fmt.Errorf("commit run listing transaction: %w", err)
	}

	return result, nil
}

func (m RunModel) GetRun(ctx context.Context, runID uuid.UUID) (RunDetails, error) {
	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	tx, err := m.DB.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})

	if err != nil {
		return RunDetails{}, fmt.Errorf("begin run details transaction: %w", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), m.queryTimeout)
		defer cleanupCancel()

		_ = tx.Rollback(cleanupCtx)
	}()

	details := RunDetails{Attempts: make([]AttemptDetails, 0)}

	runQuery := `
SELECT id, repository, branch, commit_sha, status,
       active_attempt_id, cancel_requested_at, retry_of_run_id, created_at
FROM runs
WHERE id = $1`

	err = tx.QueryRow(ctx, runQuery, runID).Scan(
		&details.ID, &details.Repository, &details.Branch, &details.CommitSHA, &details.Status,
		&details.ActiveAttemptID, &details.CancelRequestedAt, &details.RetryOfRunID, &details.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return RunDetails{}, domain.ErrNotFound
	}

	if err != nil {
		return RunDetails{}, fmt.Errorf("get run: %w", err)
	}

	attemptQuery := `
SELECT id, run_id, number, worker_id, lease_expires_at,
       started_at, finished_at, status, COALESCE(failure_kind, '')
FROM attempts
WHERE run_id = $1
ORDER BY number`

	rows, err := tx.Query(ctx, attemptQuery, runID)

	if err != nil {
		return RunDetails{}, fmt.Errorf("list run attempts: %w", err)
	}

	defer rows.Close()

	attemptIndexes := make(map[uuid.UUID]int)

	for rows.Next() {
		attempt := AttemptDetails{Steps: make([]domain.Step, 0)}

		if err := rows.Scan(
			&attempt.ID, &attempt.RunID, &attempt.Number, &attempt.WorkerID,
			&attempt.LeaseExpiresAt, &attempt.StartedAt, &attempt.FinishedAt,
			&attempt.Status, &attempt.FailureKind,
		); err != nil {
			return RunDetails{}, fmt.Errorf("scan run attempt: %w", err)
		}

		attemptIndexes[attempt.ID] = len(details.Attempts)
		details.Attempts = append(details.Attempts, attempt)
	}

	if err := rows.Err(); err != nil {
		return RunDetails{}, fmt.Errorf("read run attempts: %w", err)
	}

	rows.Close()

	stepQuery := `
SELECT s.attempt_id, s.step_index, s.name, s.command, s.status,
       s.started_at, s.finished_at, s.exit_code
FROM steps AS s
JOIN attempts AS a ON a.id = s.attempt_id
WHERE a.run_id = $1
ORDER BY a.number, s.step_index`

	rows, err = tx.Query(ctx, stepQuery, runID)

	if err != nil {
		return RunDetails{}, fmt.Errorf("list run steps: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var step domain.Step

		if err := rows.Scan(
			&step.AttemptID, &step.Index, &step.Name, &step.Command, &step.Status,
			&step.StartedAt, &step.FinishedAt, &step.ExitCode,
		); err != nil {
			return RunDetails{}, fmt.Errorf("scan run step: %w", err)
		}

		index := attemptIndexes[step.AttemptID]
		details.Attempts[index].Steps = append(details.Attempts[index].Steps, step)
	}

	if err := rows.Err(); err != nil {
		return RunDetails{}, fmt.Errorf("read run steps: %w", err)
	}

	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return RunDetails{}, fmt.Errorf("commit run details transaction: %w", err)
	}

	return details, nil
}
