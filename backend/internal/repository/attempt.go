package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AttemptModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}

func (m AttemptModel) IsInactive(ctx context.Context, attemptID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	query := `
	SELECT EXISTS (
	    SELECT 1 FROM attempts
	    WHERE id = $1 AND status <> 'running'
	)`

	var inactive bool

	if err := m.DB.QueryRow(ctx, query, attemptID).Scan(&inactive); err != nil {
		return false, fmt.Errorf("check inactive attempt: %w", err)
	}

	return inactive, nil
}
