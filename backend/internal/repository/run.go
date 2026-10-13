package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RunModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}
