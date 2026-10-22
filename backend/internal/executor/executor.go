package executor

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type ExecutionSpec struct {
	Image     string
	Workspace string
	AttemptID uuid.UUID
}

type Environment struct {
	ContainerID string
}

type ManagedEnvironment struct {
	Environment
	AttemptID uuid.UUID
}

type Executor interface {
	Prepare(ctx context.Context, spec ExecutionSpec) (Environment, error)
	Exec(ctx context.Context, env Environment, command string, output io.Writer) (int, error)
	Destroy(ctx context.Context, env Environment) error
	ListManaged(ctx context.Context) ([]ManagedEnvironment, error)
}
