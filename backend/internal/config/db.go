package config

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenDB(dsn string, queryDuration time.Duration, ctx context.Context) (*pgxpool.Pool, error) {
	connectCtx, cancel := context.WithTimeout(ctx, queryDuration)

	defer cancel()

	db, err := pgxpool.New(connectCtx, dsn)

	if err != nil {
		return nil, err
	}

	if err := db.Ping(connectCtx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
