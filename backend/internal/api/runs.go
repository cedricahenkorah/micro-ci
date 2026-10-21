package api

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
)

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Repository string `json:"repository"`
		Branch     string `json:"branch"`
		CommitSHA  string `json:"commit_sha"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		s.clientError(w, http.StatusBadRequest, "invalid run payload")
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.clientError(w, http.StatusBadRequest, "body must contain a single JSON object")
		return
	}

	input.Repository = strings.TrimSpace(input.Repository)
	input.Branch = strings.TrimSpace(input.Branch)
	input.CommitSHA = strings.ToLower(strings.TrimSpace(input.CommitSHA))

	if filepath.IsAbs(input.Repository) {
		info, err := os.Stat(input.Repository)

		if err != nil || !info.IsDir() {
			s.clientError(w, http.StatusBadRequest, "local repository must be an accessible directory")
			return
		}

		input.Repository = filepath.Clean(input.Repository)
	} else {
		allowed := s.app.Config.GitHub.Repository

		if allowed == "" || !strings.EqualFold(
			strings.TrimSuffix(input.Repository, ".git"),
			strings.TrimSuffix(allowed, ".git"),
		) {
			s.clientError(w, http.StatusBadRequest, "repository must be the configured GitHub URL or an absolute local path")
			return
		}

		input.Repository = allowed
	}

	if input.Branch == "" {
		s.clientError(w, http.StatusBadRequest, "branch is required")
		return
	}

	_, err := hex.DecodeString(input.CommitSHA)

	if err != nil || len(input.CommitSHA) != 40 || input.CommitSHA == strings.Repeat("0", 40) {
		s.clientError(w, http.StatusBadRequest, "commit_sha must be a full 40-character hexadecimal commit SHA")
		return
	}

	run, err := s.app.Models.Runs.CreateRun(r.Context(), repository.CreateRunInput{
		Repository: input.Repository,
		Branch:     input.Branch,
		CommitSHA:  input.CommitSHA,
	})

	if err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.apiResponse(w, http.StatusCreated, run, "", nil); err != nil {
		s.serverError(w, err)
	}
}
