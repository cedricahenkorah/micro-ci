package worker

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/cedricahenkorah/micro-ci/backend/internal/executor"
	"github.com/cedricahenkorah/micro-ci/backend/internal/pipeline"
	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
)

type executionResult struct {
	Outcome repository.Outcome
	Err     error
}

func Run(ctx context.Context, app *config.Application) error {
	stopMargin := app.Config.Worker.CleanupTimeout + 3*time.Second

	if app.Config.Worker.HeartbeatInterval >= app.Config.Worker.LeaseDuration-stopMargin {
		return fmt.Errorf("leaseDuration must exceed heartbeatInterval plus %s cleanup margin", stopMargin)
	}

	workerID := rand.Text()

	app.InfoLog.Printf(
		"Worker started: id=%s concurrency=%d",
		workerID,
		app.Config.Worker.Concurrency,
	)

	var wg sync.WaitGroup

	createWorkspaceErr := os.MkdirAll(app.Config.Worker.WorkspaceRoot, 0755)

	if createWorkspaceErr != nil {
		return fmt.Errorf("create workspace root: %w", createWorkspaceErr)
	}

	wg.Go(func() {
		cleanupOrphans(ctx, app)
	})

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
			"Run claimed: worker=%s slot=%d run=%s attempt_id=%s",
			workerID, slot, claim.Run.ID, claim.AttemptID,
		)

		processClaimErr := processClaim(ctx, app, *claim)

		if ctx.Err() != nil {
			return
		}

		if processClaimErr != nil {
			if errors.Is(processClaimErr, domain.ErrLeaseLost) {
				app.InfoLog.Printf(
					"Lease lost: run=%s attempt_id=%s",
					claim.Run.ID, claim.AttemptID,
				)
			} else {
				app.ErrorLog.Printf(
					"Processing failed: run=%s attempt_id=%s error=%v",
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
	stopMargin := app.Config.Worker.CleanupTimeout + 3*time.Second

	stopAt := claim.LeaseExpiresAt.Add(-stopMargin)

	if !time.Now().Before(stopAt) {
		return domain.ErrLeaseLost
	}

	leaseCtx, cancelLease := context.WithCancelCause(ctx)

	defer cancelLease(context.Canceled)

	leaseTimer := time.AfterFunc(time.Until(stopAt), func() {
		cancelLease(domain.ErrLeaseLost)
	})

	defer leaseTimer.Stop()

	workCtx, cancelWork := context.WithCancel(leaseCtx)

	results := make(chan executionResult, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)

		outcome, err := executeClaim(workCtx, leaseCtx, app, claim)
		results <- executionResult{Outcome: outcome, Err: err}
	}()

	defer func() {
		cancelWork()
		<-done
	}()

	heartbeat := time.NewTicker(app.Config.Worker.HeartbeatInterval)
	defer heartbeat.Stop()

	cancelRequested := false

	for {
		select {
		case <-leaseCtx.Done():
			return context.Cause(leaseCtx)

		case <-heartbeat.C:
			control, err := app.Models.Runs.Heartbeat(
				leaseCtx,
				claim,
				app.Config.Worker.LeaseDuration,
			)

			if leaseCtx.Err() != nil {
				return context.Cause(leaseCtx)
			}

			if err != nil {
				if errors.Is(err, domain.ErrLeaseLost) {
					cancelLease(domain.ErrLeaseLost)
					return err
				}

				app.ErrorLog.Printf(
					"Heartbeat failed: run=%s attempt_id=%s error=%v",
					claim.Run.ID, claim.AttemptID, err,
				)
				continue
			}

			nextStopAt := control.LeaseExpiresAt.Add(-stopMargin)

			if !time.Now().Before(stopAt) || !time.Now().Before(nextStopAt) {
				cancelLease(domain.ErrLeaseLost)
				return domain.ErrLeaseLost
			}

			if !leaseTimer.Reset(time.Until(nextStopAt)) {
				cancelLease(domain.ErrLeaseLost)
				return domain.ErrLeaseLost
			}

			stopAt = nextStopAt

			if control.CancelRequested {
				cancelRequested = true
				cancelWork()
			}

		case result := <-results:
			<-done

			if leaseCtx.Err() != nil {
				return context.Cause(leaseCtx)
			}

			if errors.Is(result.Err, domain.ErrLeaseLost) {
				return result.Err
			}

			if cancelRequested {
				result.Outcome = repository.Outcome{
					Status:      domain.AttemptStatusCancelled,
					FailureKind: domain.FailureKindCancelled,
				}
			} else if result.Err != nil {
				return result.Err
			}

			if result.Outcome.Status == domain.AttemptStatusFailed {
				skipErr := app.Models.Steps.SkipPendingSteps(leaseCtx, claim)

				if skipErr != nil {
					return fmt.Errorf("skip pending steps: %w", skipErr)
				}
			}

			return app.Models.Runs.Finish(leaseCtx, claim, result.Outcome)
		}
	}
}

func executeClaim(
	ctx, leaseCtx context.Context,
	app *config.Application,
	claim repository.Claim,
) (outcome repository.Outcome, err error) {
	parentCtx := ctx

	preparationDeadline := claim.StartedAt.Add(app.Config.Worker.PreparationTimeout)

	ctx, cancelPreparation := context.WithDeadline(parentCtx, preparationDeadline)
	defer cancelPreparation()

	defer func() {
		if err == context.DeadlineExceeded && parentCtx.Err() == nil {
			outcome = repository.Outcome{
				Status:      domain.AttemptStatusTimedOut,
				FailureKind: domain.FailureKindTimeout,
			}

			err = nil
		}
	}()

	ctxErr := ctx.Err()

	if ctxErr != nil {
		return repository.Outcome{}, ctxErr
	}

	workspace, workspaceErr := os.MkdirTemp(
		app.Config.Worker.WorkspaceRoot,
		fmt.Sprintf("attempt-%s-", claim.AttemptID),
	)

	if workspaceErr != nil {
		app.ErrorLog.Printf(
			"Create workspace failed: run=%s attempt_id=%s error=%v",
			claim.Run.ID, claim.AttemptID, workspaceErr,
		)

		return repository.Outcome{
			Status:      domain.AttemptStatusFailed,
			FailureKind: domain.FailureKindInfrastructure,
		}, nil
	}

	removeWorkspace := true

	defer func() {
		if !removeWorkspace {
			app.ErrorLog.Printf(
				"Workspace retained: run=%s attempt_id=%s path=%s",
				claim.Run.ID, claim.AttemptID, workspace,
			)
			return
		}

		removeErr := os.RemoveAll(workspace)

		if removeErr != nil {
			app.ErrorLog.Printf(
				"Remove workspace failed: run=%s attempt_id=%s path=%s error=%v",
				claim.Run.ID, claim.AttemptID, workspace, removeErr,
			)
		}
	}()

	checkoutErr := executor.Checkout(
		ctx,
		claim.Run.Repository,
		claim.Run.CommitSHA,
		workspace,
	)

	if checkoutErr != nil {
		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		app.ErrorLog.Printf(
			"Checkout failed: run=%s attempt_id=%s error=%v",
			claim.Run.ID, claim.AttemptID, checkoutErr,
		)

		return repository.Outcome{
			Status:      domain.AttemptStatusFailed,
			FailureKind: domain.FailureKindCheckout,
		}, nil
	}

	pipelinePath := filepath.Join(workspace, ".micro-ci.yml")

	file, openErr := os.Open(pipelinePath)

	if openErr != nil {
		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		app.ErrorLog.Printf("Failed to open %s: run=%s attempt_id=%s error=%v", pipelinePath, claim.Run.ID, claim.AttemptID, openErr)

		return repository.Outcome{
			Status:      domain.AttemptStatusFailed,
			FailureKind: domain.FailureKindConfig,
		}, nil
	}

	defer file.Close()

	pipelineConfig, parseErr := pipeline.Parse(file)

	if parseErr != nil {
		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		app.ErrorLog.Printf("Failed to parse %s: run=%s attempt_id=%s error=%v", pipelinePath, claim.Run.ID, claim.AttemptID, parseErr)

		return repository.Outcome{
			Status:      domain.AttemptStatusFailed,
			FailureKind: domain.FailureKindConfig,
		}, nil
	}

	pipelineDeadline := claim.StartedAt.Add(time.Duration(pipelineConfig.TimeoutSeconds) * time.Second)

	pipelineCtx, cancelPipeline := context.WithDeadline(parentCtx, pipelineDeadline)
	defer cancelPipeline()

	ctx, cancelSetup := context.WithDeadline(pipelineCtx, preparationDeadline)
	defer cancelSetup()

	cancelPreparation()

	if ctx.Err() != nil {
		return repository.Outcome{}, ctx.Err()
	}

	app.InfoLog.Printf(
		"Pipeline loaded: run=%s attempt_id=%s image=%s steps=%d",
		claim.Run.ID, claim.AttemptID,
		pipelineConfig.Image, len(pipelineConfig.Steps),
	)

	stepInputs := make([]repository.CreateStepInput, len(pipelineConfig.Steps))

	for i, step := range pipelineConfig.Steps {
		stepInputs[i] = repository.CreateStepInput{
			Name:    step.Name,
			Command: step.Command,
		}
	}

	createStepsErr := app.Models.Steps.CreateSteps(ctx, claim, stepInputs)

	if createStepsErr != nil {
		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		return repository.Outcome{}, fmt.Errorf(
			"create steps: %w",
			createStepsErr,
		)
	}

	removeWorkspace = false

	env, prepareErr := app.Executor.Prepare(ctx, executor.ExecutionSpec{
		Image:     pipelineConfig.Image,
		Workspace: workspace,
		AttemptID: claim.AttemptID,
	})

	if prepareErr != nil {
		removeWorkspace = env.ContainerID == ""

		app.ErrorLog.Printf(
			"Prepare failed: run=%s attempt_id=%s error=%v",
			claim.Run.ID, claim.AttemptID, prepareErr,
		)

		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		return repository.Outcome{
			Status:      domain.AttemptStatusFailed,
			FailureKind: domain.FailureKindInfrastructure,
		}, nil
	}

	defer func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(
			context.Background(),
			app.Config.Worker.CleanupTimeout,
		)

		defer cancelCleanup()

		destroyErr := app.Executor.Destroy(cleanupCtx, env)

		if destroyErr != nil {
			app.ErrorLog.Printf(
				"Destroy failed: run=%s attempt_id=%s container=%s error=%v",
				claim.Run.ID, claim.AttemptID, env.ContainerID, destroyErr,
			)

			if err == nil && outcome.Status == domain.AttemptStatusSucceeded {
				outcome = repository.Outcome{
					Status:      domain.AttemptStatusFailed,
					FailureKind: domain.FailureKindInfrastructure,
				}
			}
			return
		}

		removeWorkspace = true
	}()

	setupErr := ctx.Err()

	cancelSetup()

	ctx = pipelineCtx

	if setupErr != nil {
		return repository.Outcome{}, setupErr
	}

	for i, step := range pipelineConfig.Steps {
		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		startStepErr := app.Models.Steps.StartStep(ctx, claim, i)

		if startStepErr != nil {
			if ctx.Err() != nil {
				return repository.Outcome{}, ctx.Err()
			}

			return repository.Outcome{}, fmt.Errorf(
				"start step %d: %w",
				i+1,
				startStepErr,
			)
		}

		app.InfoLog.Printf(
			"Step started: run=%s attempt_id=%s step=%d name=%q",
			claim.Run.ID, claim.AttemptID, i+1, step.Name,
		)

		logWriter := NewLogWriter(
			leaseCtx,
			app.Models.Events.DB,
			app.Config.Db.QueryTimeout,
			claim,
			i,
		)

		commandCtx, cancelCommand := context.WithCancel(ctx)
		flushDone := make(chan struct{})

		go func() {
			defer close(flushDone)

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-commandCtx.Done():
					return
				case <-ticker.C:
					if err := logWriter.Flush(); err != nil {
						cancelCommand()
						return
					}
				}
			}
		}()

		exitCode, execErr := app.Executor.Exec(
			commandCtx,
			env,
			step.Command,
			io.MultiWriter(logWriter, os.Stdout),
		)

		cancelCommand()
		<-flushDone

		flushErr := logWriter.Flush()

		if errors.Is(flushErr, domain.ErrLeaseLost) {
			return repository.Outcome{}, flushErr
		}

		if flushErr != nil {
			app.ErrorLog.Printf(
				"Log flush failed: run=%s attempt_id=%s step=%d error=%v",
				claim.Run.ID, claim.AttemptID, i+1, flushErr,
			)

			execErr = fmt.Errorf("flush step %d logs: %w", i+1, flushErr)
		}

		if ctx.Err() != nil {
			return repository.Outcome{}, ctx.Err()
		}

		stepResult := repository.StepResult{
			Status: domain.StepStatusFailed,
		}

		if execErr == nil {
			stepResult.ExitCode = &exitCode

			if exitCode == 0 {
				stepResult.Status = domain.StepStatusSucceeded
			}
		}

		finishStepErr := app.Models.Steps.FinishStep(
			ctx,
			claim,
			i,
			stepResult,
		)

		if finishStepErr != nil {
			if ctx.Err() != nil {
				return repository.Outcome{}, ctx.Err()
			}

			return repository.Outcome{}, fmt.Errorf(
				"finish step %d: %w",
				i+1,
				finishStepErr,
			)
		}

		if execErr != nil {
			app.ErrorLog.Printf(
				"Step execution failed: run=%s attempt_id=%s step=%d error=%v",
				claim.Run.ID, claim.AttemptID, i+1, execErr,
			)

			return repository.Outcome{
				Status:      domain.AttemptStatusFailed,
				FailureKind: domain.FailureKindInfrastructure,
			}, nil
		}

		if exitCode != 0 {
			app.ErrorLog.Printf(
				"Step failed: run=%s attempt_id=%s step=%d exit_code=%d",
				claim.Run.ID, claim.AttemptID, i+1, exitCode,
			)

			return repository.Outcome{
				Status:      domain.AttemptStatusFailed,
				FailureKind: domain.FailureKindCommand,
			}, nil
		}

		app.InfoLog.Printf(
			"Step succeeded: run=%s attempt_id=%s step=%d name=%q",
			claim.Run.ID, claim.AttemptID, i+1, step.Name,
		)
	}

	if ctx.Err() != nil {
		return repository.Outcome{}, ctx.Err()
	}

	return repository.Outcome{
		Status: domain.AttemptStatusSucceeded,
	}, nil
}
