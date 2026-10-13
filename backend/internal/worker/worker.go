package worker

import (
	"context"
	"log"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
)

func Run(ctx context.Context, app *config.Application) error {
	log.Println("Worker started")

	<-ctx.Done()

	log.Println("Worker stopped")
	return nil
}
