package executor

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DockerExecutor struct {
	CleanupTimeout time.Duration
}

func (d DockerExecutor) Prepare(
	ctx context.Context,
	spec ExecutionSpec,
) (env Environment, prepareErr error) {
	if strings.TrimSpace(spec.Image) == "" {
		return Environment{}, fmt.Errorf("image is required")
	}

	if spec.AttemptID <= 0 {
		return Environment{}, fmt.Errorf("attemptID must be a positive integer")
	}

	if !filepath.IsAbs(spec.Workspace) {
		return Environment{}, fmt.Errorf("workspace path must be absolute: %q", spec.Workspace)
	}

	info, err := os.Stat(spec.Workspace)

	if err != nil {
		return Environment{}, fmt.Errorf("access workspace: %w", err)
	}

	if !info.IsDir() {
		return Environment{}, fmt.Errorf("workspace must be a directory: %s", spec.Workspace)
	}

	_, dockerErr := runDocker(ctx, "pull", spec.Image)

	if dockerErr != nil {
		return Environment{}, fmt.Errorf("pull image %q: %w", spec.Image, dockerErr)
	}

	containerName := fmt.Sprintf(
		"micro-ci-attempt-%d-%s",
		spec.AttemptID,
		strings.ToLower(rand.Text()),
	)

	defer func() {
		if prepareErr == nil {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.Background(),
			d.CleanupTimeout,
		)

		defer cancel()

		_, cleanupErr := runDocker(
			cleanupCtx,
			"rm", "--force", containerName,
		)

		if cleanupErr == nil {
			return
		}

		if strings.Contains(cleanupErr.Error(), "No such container:") {
			return
		}

		env = Environment{ContainerID: containerName}

		prepareErr = errors.Join(
			prepareErr,
			fmt.Errorf("remove partial container %q: %w", containerName, cleanupErr),
		)
	}()

	containerID, createErr := runDocker(
		ctx,
		"create",
		"--name", containerName,
		"--label", "micro-ci.managed=true",
		"--label", fmt.Sprintf("micro-ci.attempt-id=%d", spec.AttemptID),
		"--init",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/workspace", spec.Workspace),
		"--workdir", "/workspace",
		"--entrypoint", "sh",
		spec.Image,
		"-c", "while :; do sleep 3600; done",
	)

	if createErr != nil {
		return Environment{}, fmt.Errorf("create container %q: %w", containerName, createErr)
	}

	_, startErr := runDocker(ctx, "start", containerID)

	if startErr != nil {
		return Environment{}, fmt.Errorf("start container %q: %w", containerID, startErr)
	}

	inspectOutput, inspectOutputErr := runDocker(ctx, "inspect", "--format", "{{.State.Running}}", containerID)

	if inspectOutputErr != nil {
		return Environment{}, fmt.Errorf("inspect container %q: %w", containerID, inspectOutputErr)
	}

	if inspectOutput != "true" {
		return Environment{}, fmt.Errorf("container %q is not running", containerID)
	}

	return Environment{ContainerID: containerID}, nil
}

func (d DockerExecutor) Destroy(
	ctx context.Context,
	env Environment,
) error {
	if strings.TrimSpace(env.ContainerID) == "" {
		return fmt.Errorf("containerID is required")
	}

	_, removeContainerErr := runDocker(ctx, "rm", "--force", env.ContainerID)

	if removeContainerErr != nil {
		if strings.Contains(removeContainerErr.Error(), "No such container:") {
			return nil
		}

		return fmt.Errorf("remove container %q: %w", env.ContainerID, removeContainerErr)
	}

	return nil
}

func (d DockerExecutor) Exec(
	ctx context.Context,
	env Environment,
	command string,
	output io.Writer,
) (int, error) {
	if strings.TrimSpace(env.ContainerID) == "" {
		return -1, fmt.Errorf("containerID is required")
	}

	if strings.TrimSpace(command) == "" {
		return -1, fmt.Errorf("command is required")
	}

	if ctx.Err() != nil {
		return -1, ctx.Err()
	}

	if output == nil {
		output = io.Discard
	}

	cmd := exec.CommandContext(
		ctx,
		"docker", "exec",
		"--workdir", "/workspace",
		env.ContainerID,
		"sh", "-c", command,
	)

	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = time.Second

	runErr := cmd.Run()

	if ctx.Err() != nil {
		return -1, ctx.Err()
	}

	if runErr == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError

	if errors.As(runErr, &exitErr) {
		if exitCode := exitErr.ExitCode(); exitCode >= 0 {
			return exitCode, nil
		}
	}

	return -1, fmt.Errorf(
		"execute command in container %q: %w",
		env.ContainerID,
		runErr,
	)
}

func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.WaitDelay = time.Second

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, outputErr := cmd.Output()

	if outputErr != nil {
		if ctx.Err() != nil {
			outputErr = ctx.Err()
		}

		return "", fmt.Errorf("docker %s: %w; stderr: %s",
			strings.Join(args, " "),
			outputErr,
			strings.TrimSpace(stderr.String()),
		)
	}

	return strings.TrimSpace(string(output)), nil
}
