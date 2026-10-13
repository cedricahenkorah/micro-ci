package worker

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
)

func Run(ctx context.Context, app *config.Application) error {
	workerID := rand.Text()

	app.InfoLog.Printf(
		"Worker started: id=%s concurrency=%d",
		workerID,
		app.Config.Worker.Concurrency,
	)

	var wg sync.WaitGroup

	for slot := 1; slot <= app.Config.Worker.Concurrency; slot++ {
		wg.Add(1)

		go func(slot int) {
			defer wg.Done()
			poll(ctx, app, workerID, slot)
		}(slot)
	}

	wg.Wait()

	app.InfoLog.Printf("Worker stopped: id=%s", workerID)
	return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)

	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func poll(
	ctx context.Context,
	app *config.Application,
	workerID string,
	slot int,
) {
	for ctx.Err() == nil {
		claim, claimErr := app.Models.Runs.Claim(
			ctx,
			workerID,
			app.Config.Worker.LeaseDuration,
		)

		if ctx.Err() != nil {
			return
		}

		if claimErr != nil {
			app.ErrorLog.Printf(
				"Claim failed: worker=%s slot=%d error=%v",
				workerID, slot, claimErr,
			)

			if !wait(ctx, app.Config.Worker.PollInterval) {
				return
			}

			continue
		}

		if claim == nil {
			if !wait(ctx, app.Config.Worker.PollInterval) {
				return
			}

			continue
		}

		app.InfoLog.Printf(
			"Run claimed: worker=%s slot=%d run=%d attempt_id=%d",
			workerID, slot, claim.Run.ID, claim.AttemptID,
		)

		processClaimErr := processClaim(ctx, app, *claim)

		if ctx.Err() != nil {
			return
		}

		if processClaimErr != nil {
			if errors.Is(processClaimErr, domain.ErrLeaseLost) {
				app.InfoLog.Printf(
					"Lease lost: run=%d attempt_id=%d",
					claim.Run.ID, claim.AttemptID,
				)
			} else {
				app.ErrorLog.Printf(
					"Processing failed: run=%d attempt_id=%d error=%v",
					claim.Run.ID, claim.AttemptID, processClaimErr,
				)
			}

			if !wait(ctx, app.Config.Worker.PollInterval) {
				return
			}
		}
	}
}

func processClaim(
	ctx context.Context,
	app *config.Application,
	claim repository.Claim,
) error {
	work := time.NewTimer(10 * time.Second)
	defer work.Stop()

	heartbeat := time.NewTicker(app.Config.Worker.HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-heartbeat.C:
			control, err := app.Models.Runs.Heartbeat(
				ctx,
				claim,
				app.Config.Worker.LeaseDuration,
			)

			if err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}

			if control.CancelRequested {
				return app.Models.Runs.Finish(ctx, claim, repository.Outcome{
					Status:      domain.AttemptStatusCancelled,
					FailureKind: domain.FailureKindCancelled,
				})
			}

		case <-work.C:
			return app.Models.Runs.Finish(ctx, claim, repository.Outcome{
				Status: domain.AttemptStatusSucceeded,
			})
		}
	}
}
