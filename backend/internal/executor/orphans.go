package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (d DockerExecutor) ListManaged(ctx context.Context) ([]ManagedEnvironment, error) {
	output, err := runDocker(
		ctx, "ps", "--all", "--no-trunc",
		"--filter", "label=micro-ci.managed=true",
		"--format", `{{.ID}} {{.Label "micro-ci.attempt-id"}}`,
	)

	if err != nil {
		return nil, fmt.Errorf("list managed containers: %w", err)
	}

	var environments []ManagedEnvironment

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)

		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid managed container entry: %q", line)
		}

		attemptID, err := uuid.Parse(fields[1])

		if err != nil || attemptID == uuid.Nil {
			return nil, fmt.Errorf("invalid attempt ID for container %s: %q", fields[0], fields[1])
		}

		environments = append(environments, ManagedEnvironment{
			Environment: Environment{ContainerID: fields[0]},
			AttemptID:   attemptID,
		})
	}

	return environments, nil
}
