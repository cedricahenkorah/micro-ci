package api

import (
	"context"
	"log"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
)

func Run(ctx context.Context, app *config.Application) error {
	log.Println("API started")

	<-ctx.Done()

	log.Println("API stopped")
	return nil
}
