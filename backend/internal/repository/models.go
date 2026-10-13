package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Models struct {
}

func New(db *pgxpool.Pool, queryTimeout time.Duration) Models {
	return Models{}
}
