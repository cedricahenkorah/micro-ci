package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
	"github.com/cedricahenkorah/micro-ci/backend/internal/executor"
	"github.com/google/uuid"
)

type attemptResources struct {
	Containers []executor.Environment
	Workspaces []string
}

func cleanupOrphans(ctx context.Context, app *config.Application) {
	for ctx.Err() == nil {
		err := SweepOrphans(ctx, app)

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			app.ErrorLog.Printf("Orphan cleanup failed: %v", err)
		}

		if !wait(ctx, 30*time.Second) {
			return
		}
	}
}

func SweepOrphans(ctx context.Context, app *config.Application) error {
	entries, err := os.ReadDir(app.Config.Worker.WorkspaceRoot)

	if err != nil {
		return fmt.Errorf("list attempt workspaces: %w", err)
	}

	resources := make(map[uuid.UUID]*attemptResources)

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "attempt-") {
			continue
		}

		name := strings.TrimPrefix(entry.Name(), "attempt-")

		if len(name) <= 37 || name[36] != '-' {
			continue
		}

		attemptID, err := uuid.Parse(name[:36])

		if err != nil || attemptID == uuid.Nil {
			continue
		}

		if resources[attemptID] == nil {
			resources[attemptID] = &attemptResources{}
		}

		resources[attemptID].Workspaces = append(
			resources[attemptID].Workspaces,
			filepath.Join(app.Config.Worker.WorkspaceRoot, entry.Name()),
		)
	}

	listCtx, cancelList := context.WithTimeout(ctx, app.Config.Worker.CleanupTimeout)

	containers, err := app.Executor.ListManaged(listCtx)

	cancelList()

	if err != nil {
		return err
	}

	for _, container := range containers {
		if resources[container.AttemptID] == nil {
			resources[container.AttemptID] = &attemptResources{}
		}

		resources[container.AttemptID].Containers = append(
			resources[container.AttemptID].Containers, container.Environment,
		)
	}

	var cleanupErr error

	for attemptID, resource := range resources {
		if ctx.Err() != nil {
			return errors.Join(cleanupErr, ctx.Err())
		}

		inactive, err := app.Models.Attempts.IsInactive(ctx, attemptID)

		if err != nil {
			return errors.Join(cleanupErr, fmt.Errorf("check attempt %s: %w", attemptID, err))
		}

		if !inactive {
			continue
		}

		containersRemoved := true

		for _, container := range resource.Containers {
			cleanupCtx, cancelCleanup := context.WithTimeout(ctx, app.Config.Worker.CleanupTimeout)

			err := app.Executor.Destroy(cleanupCtx, container)

			cancelCleanup()

			if err != nil {
				containersRemoved = false
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("clean attempt %s: %w", attemptID, err))
			}
		}

		if !containersRemoved {
			continue
		}

		for _, workspace := range resource.Workspaces {
			if ctx.Err() != nil {
				return errors.Join(cleanupErr, ctx.Err())
			}

			if err := os.RemoveAll(workspace); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove workspace %s: %w", workspace, err))
			}
		}
	}

	return cleanupErr
}
