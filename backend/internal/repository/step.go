package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StepModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}
