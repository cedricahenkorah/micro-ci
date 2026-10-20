package executor

import (
	"context"
	"io"
)

type ExecutionSpec struct {
	Image     string
	Workspace string
	AttemptID int64
}

type Environment struct {
	ContainerID string
}

type Executor interface {
	Prepare(ctx context.Context, spec ExecutionSpec) (Environment, error)
	Exec(ctx context.Context, env Environment, command string, output io.Writer) (int, error)
	Destroy(ctx context.Context, env Environment) error
}
