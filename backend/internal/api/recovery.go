package api

import (
	"context"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
)

func recoverExpired(ctx context.Context, app *config.Application) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := app.Models.Runs.RecoverExpired(ctx, 2)

			if ctx.Err() != nil {
				return
			}

			if err != nil {
				app.ErrorLog.Printf("Recover expired runs failed: %v", err)
			}
		}
	}
}
