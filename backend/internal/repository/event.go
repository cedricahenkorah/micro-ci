package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}

type CreateEventInput struct {
	AttemptID *int64
	Type      domain.EventType
	StepIndex *int
	Payload   json.RawMessage
}

// The caller owns the transaction and any worker ownership checks.
func (m EventModel) AppendEvent(ctx context.Context, tx pgx.Tx, runID int64, event CreateEventInput) error {
	if tx == nil {
		return errors.New("transaction is required")
	}

	if runID <= 0 {
		return errors.New("runID must be positive")
	}

	if event.AttemptID != nil && *event.AttemptID <= 0 {
		return errors.New("attemptID must be positive")
	}

	if event.Type != domain.EventTypeRunStatus &&
		event.Type != domain.EventTypeAttemptStatus &&
		event.Type != domain.EventTypeStepStatus &&
		event.Type != domain.EventTypeLog &&
		event.Type != domain.EventTypeLogTruncated {
		return fmt.Errorf("invalid event type %q", event.Type)
	}

	if event.StepIndex != nil && *event.StepIndex < 0 {
		return errors.New("step index must not be negative")
	}

	if event.StepIndex != nil && event.AttemptID == nil {
		return errors.New("step event requires an attemptID")
	}

	if !json.Valid(event.Payload) {
		return errors.New("event payload must be valid JSON")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)

	defer cancel()

	var lockedRunID int64

	err := tx.QueryRow(ctx, `
		SELECT id FROM runs
		WHERE id = $1
		FOR UPDATE`, runID).Scan(&lockedRunID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}

	if err != nil {
		return fmt.Errorf("lock event run: %w", err)
	}

	var sequence int64

	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence), 0) FROM events WHERE run_id = $1`, runID).Scan(&sequence)

	if err != nil {
		return fmt.Errorf("read event sequence: %w", err)
	}

	query := `
		INSERT INTO events (run_id, attempt_id, sequence, type, step_index, timestamp, payload)
		VALUES ($1, $2, $3, $4, $5, clock_timestamp(), $6)`

	_, err = tx.Exec(ctx, query, runID, event.AttemptID, sequence+1, event.Type, event.StepIndex, event.Payload)

	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

func (m EventModel) ReadEvents(ctx context.Context, runID, afterSequence int64, limit int) ([]domain.Event, error) {
	if runID <= 0 {
		return nil, errors.New("runID must be positive")
	}

	if afterSequence < 0 {
		return nil, errors.New("afterSequence must not be negative")
	}

	if limit <= 0 {
		return nil, errors.New("limit must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	query := `
		SELECT run_id, attempt_id, sequence, type, step_index, timestamp, payload
		FROM events
		WHERE run_id = $1 AND sequence > $2
		ORDER BY sequence
		LIMIT $3`

	rows, err := m.DB.Query(ctx, query, runID, afterSequence, limit)

	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}

	defer rows.Close()

	events := make([]domain.Event, 0)

	for rows.Next() {
		var event domain.Event

		if err := rows.Scan(
			&event.RunID, &event.AttemptID, &event.Sequence, &event.Type,
			&event.StepIndex, &event.Timestamp, &event.Payload,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}

	return events, nil
}
