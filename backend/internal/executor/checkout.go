package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Checkout(
	ctx context.Context,
	repository string,
	sha string,
	destination string,
) error {
	if strings.TrimSpace(repository) == "" {
		return fmt.Errorf("repository URL is required")
	}

	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("checkout destination directory is required")
	}

	commitSha := strings.ToLower(strings.TrimSpace(sha))

	if len(commitSha) != 40 {
		return fmt.Errorf("commit SHA must be exactly 40 hexadecimal characters")
	}

	_, decodeShaErr := hex.DecodeString(commitSha)

	if decodeShaErr != nil {
		return fmt.Errorf("commit SHA must contain only hexadecimal characters")
	}

	_, initGitErr := runGit(ctx, destination, "init")

	if initGitErr != nil {
		return fmt.Errorf("initialize checkout repository: %w", initGitErr)
	}

	_, originErr := runGit(ctx, destination, "remote", "add", "origin", repository)

	if originErr != nil {
		return fmt.Errorf("add origin remote: %w", originErr)
	}

	_, fetchCommitErr := runGit(ctx, destination, "fetch", "--depth=1", "--no-tags", "origin", commitSha)

	if fetchCommitErr != nil {
		return fmt.Errorf("fetch requested commit: %w", fetchCommitErr)
	}

	_, checkoutCommitErr := runGit(ctx, destination, "checkout", "--detach", "FETCH_HEAD")

	if checkoutCommitErr != nil {
		return fmt.Errorf("check out fetched commit: %w", checkoutCommitErr)
	}

	head, commitHashErr := runGit(ctx, destination, "rev-parse", "HEAD")

	if commitHashErr != nil {
		return fmt.Errorf("resolve checked-out commit SHA: %w", commitHashErr)
	}

	if head != commitSha {
		return fmt.Errorf("checkout commit mismatch: wanted %s, got %s", commitSha, head)
	}

	return nil
}

func runGit(ctx context.Context, directory string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)

	cmd.Dir = directory

	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}

		return "", fmt.Errorf(
			"git %s: %w; output: %s",
			strings.Join(args, " "),
			err,
			strings.TrimSpace(string(output)),
		)
	}

	return strings.TrimSpace(string(output)), nil
}
