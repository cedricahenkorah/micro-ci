package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
	"github.com/go-chi/chi/v5"
)

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	limit := 20
	var page int64

	if value := r.URL.Query().Get("limit"); value != "" {
		var err error

		limit, err = strconv.Atoi(value)

		if err != nil || limit < 1 || limit > 100 {
			s.clientError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
	}

	if value := r.URL.Query().Get("page"); value != "" {
		var err error

		page, err = strconv.ParseInt(value, 10, 32)

		if err != nil || page < 0 {
			s.clientError(w, http.StatusBadRequest, "page must be a non-negative 32-bit integer")
			return
		}
	}

	result, err := s.app.Models.Runs.ListRuns(r.Context(), limit, page)

	if err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.apiResponse(w, http.StatusOK, result, "", nil); err != nil {
		s.serverError(w, err)
	}
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err != nil || runID <= 0 {
		s.clientError(w, http.StatusBadRequest, "id must be a positive run ID")
		return
	}

	details, err := s.app.Models.Runs.GetRun(r.Context(), runID)

	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, r)
		return
	}

	if err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.apiResponse(w, http.StatusOK, details, "", nil); err != nil {
		s.serverError(w, err)
	}
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err != nil || runID <= 0 {
		s.clientError(w, http.StatusBadRequest, "id must be a positive run ID")
		return
	}

	run, err := s.app.Models.Runs.RequestCancel(r.Context(), runID)

	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, r)
		return
	}

	if err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.apiResponse(w, http.StatusOK, run, "", nil); err != nil {
		s.serverError(w, err)
	}
}

func (s *Server) retryRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err != nil || runID <= 0 {
		s.clientError(w, http.StatusBadRequest, "id must be a positive run ID")
		return
	}

	run, err := s.app.Models.Runs.RetryRun(r.Context(), runID)

	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, r)
		return
	}

	if errors.Is(err, domain.ErrInvalidState) {
		s.clientError(w, http.StatusConflict, "cannot retry a queued or running run")
		return
	}

	if err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.apiResponse(w, http.StatusCreated, run, "", nil); err != nil {
		s.serverError(w, err)
	}
}

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
