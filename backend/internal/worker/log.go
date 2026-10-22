package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const logChunkSize = 16 * 1024

type LogWriter struct {
	mu           sync.Mutex
	ctx          context.Context
	db           *pgxpool.Pool
	queryTimeout time.Duration
	claim        repository.Claim
	stepIndex    int
	buffer       []byte
	err          error
}

func NewLogWriter(ctx context.Context, db *pgxpool.Pool, queryTimeout time.Duration, claim repository.Claim, stepIndex int) *LogWriter {
	return &LogWriter{
		ctx:          ctx,
		db:           db,
		queryTimeout: queryTimeout,
		claim:        claim,
		stepIndex:    stepIndex,
	}
}

func (w *LogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.err != nil {
		return 0, w.err
	}

	w.buffer = append(w.buffer, p...)

	if len(w.buffer) >= logChunkSize {
		return len(p), w.flush()
	}

	return len(p), nil
}

func (w *LogWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.flush()
}

func (w *LogWriter) flush() (err error) {
	if w.err != nil {
		return w.err
	}

	if len(w.buffer) == 0 {
		return nil
	}

	defer func() {
		w.err = err
	}()

	if w.claim.Run.ID == uuid.Nil || w.claim.AttemptID == uuid.Nil || strings.TrimSpace(w.claim.WorkerID) == "" {
		return errors.New("a valid claim is required")
	}

	if w.stepIndex < 0 {
		return errors.New("step index must not be negative")
	}

	ctx, cancel := context.WithTimeout(w.ctx, w.queryTimeout)
	defer cancel()

	tx, err := w.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin log transaction: %w", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), w.queryTimeout)
		defer cleanupCancel()
		_ = tx.Rollback(cleanupCtx)
	}()

	var lockedRunID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM runs
		WHERE id = $1 AND active_attempt_id = $2 AND status = 'running'
		FOR UPDATE`, w.claim.Run.ID, w.claim.AttemptID).Scan(&lockedRunID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}
	if err != nil {
		return fmt.Errorf("lock log run: %w", err)
	}

	var lockedAttemptID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM attempts
		WHERE id = $1 AND run_id = $2 AND worker_id = $3 AND status = 'running'
		FOR UPDATE`, w.claim.AttemptID, w.claim.Run.ID, w.claim.WorkerID).Scan(&lockedAttemptID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}
	if err != nil {
		return fmt.Errorf("lock log attempt: %w", err)
	}

	payload, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: string(w.buffer)})

	if err != nil {
		return fmt.Errorf("encode log event: %w", err)
	}

	err = repository.AppendEvent(ctx, tx, w.claim.Run.ID, repository.CreateEventInput{
		AttemptID: &w.claim.AttemptID,
		Type:      domain.EventTypeLog,
		StepIndex: &w.stepIndex,
		Payload:   payload,
	})

	if err != nil {
		return fmt.Errorf("append log event: %w", err)
	}

	err = tx.QueryRow(ctx, `
		SELECT id FROM attempts
		WHERE id = $1 AND lease_expires_at > clock_timestamp()`, w.claim.AttemptID).Scan(&lockedAttemptID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf("check log lease: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit log: %w", err)
	}

	w.buffer = w.buffer[:0]
	return nil
}
