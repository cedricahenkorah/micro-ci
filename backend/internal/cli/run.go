/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/executor"
	"github.com/spf13/cobra"
)

var apiURL string

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run <path>",
	Short: "Queue a pipeline run for a local repository's current commit",
	Long: `Read the branch and current commit from a local Git repository,
then submit a run to the micro-ci API. Use . for the current directory.

The worker executes the commit in a separate workspace. Uncommitted changes
are excluded, and no GitHub push is required. The API and worker must be running.`,
	Example: "  micro-ci run .\n  micro-ci run /path/to/project",
	Args:    cobra.ExactArgs(1),
	RunE:    runLocal,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&apiURL, "api-url", "http://localhost:4000", "Micro-ci API base URL")
}

func runLocal(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	directory, err := filepath.Abs(args[0])

	if err != nil {
		return fmt.Errorf("resolve repository path: %w", err)
	}

	root, err := executor.RunGit(ctx, directory, "rev-parse", "--show-toplevel")

	if err != nil {
		return fmt.Errorf("find Git repository: %w", err)
	}

	branch, err := executor.RunGit(ctx, root, "branch", "--show-current")

	if err != nil {
		return fmt.Errorf("read current branch: %w", err)
	}

	if branch == "" {
		return fmt.Errorf("repository has a detached HEAD; switch to a branch before starting a run")
	}

	sha, err := executor.RunGit(ctx, root, "rev-parse", "--verify", "HEAD")

	if err != nil {
		return fmt.Errorf("read current commit; create a commit before starting a run: %w", err)
	}

	input := struct {
		Repository string `json:"repository"`
		Branch     string `json:"branch"`
		CommitSHA  string `json:"commit_sha"`
	}{
		Repository: root,
		Branch:     branch,
		CommitSHA:  sha,
	}

	body, err := json.Marshal(input)

	if err != nil {
		return fmt.Errorf("encode run request: %w", err)
	}

	endpoint := strings.TrimRight(apiURL, "/") + "/api/v1/runs"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))

	if err != nil {
		return fmt.Errorf("create API request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(req)

	if err != nil {
		return fmt.Errorf("submit run to %s; check that the API is running: %w", apiURL, err)
	}

	defer response.Body.Close()

	var result struct {
		Message string `json:"message"`
		Data    struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}

	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)

	if response.StatusCode != http.StatusCreated {
		if result.Message != "" {
			return fmt.Errorf("create run: %s (%s)", result.Message, response.Status)
		}

		return fmt.Errorf("create run: API returned %s", response.Status)
	}

	if decodeErr != nil {
		return fmt.Errorf("read run response: %w", decodeErr)
	}

	if result.Data.ID <= 0 {
		return fmt.Errorf("API response did not include a valid run ID")
	}

	cmd.Printf("Run %d queued.\n", result.Data.ID)
	return nil
}
