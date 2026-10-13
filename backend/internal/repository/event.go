package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EventModel struct {
	DB           *pgxpool.Pool
	queryTimeout time.Duration
}
