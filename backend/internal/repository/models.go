package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Models struct {
	Runs     RunModel
	Attempts AttemptModel
	Steps    StepModel
	Events   EventModel
}

func New(db *pgxpool.Pool, queryTimeout time.Duration) Models {
	return Models{
		Runs:     RunModel{DB: db, queryTimeout: queryTimeout},
		Attempts: AttemptModel{DB: db, queryTimeout: queryTimeout},
		Steps:    StepModel{DB: db, queryTimeout: queryTimeout},
		Events:   EventModel{DB: db, queryTimeout: queryTimeout},
	}
}
