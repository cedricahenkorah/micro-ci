package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
)

type githubPush struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := s.app.Config.GitHub

	if cfg.Repository == "" || cfg.WebhookSecret == "" {
		s.clientError(w, http.StatusServiceUnavailable, "GitHub webhooks are not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)

	if err != nil {
		var sizeErr *http.MaxBytesError

		if errors.As(err, &sizeErr) {
			s.clientError(w, http.StatusRequestEntityTooLarge, "webhook payload is too large")
			return
		}

		s.clientError(w, http.StatusBadRequest, "could not read webhook payload")
		return
	}

	mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))

	mac.Write(body)

	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(r.Header.Get("X-Hub-Signature-256")), []byte(expected)) {
		s.clientError(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}

	if r.Header.Get("X-GitHub-Event") != "push" {
		s.apiResponse(w, http.StatusAccepted, nil, "event ignored", nil)
		return
	}

	var push githubPush

	if err := json.Unmarshal(body, &push); err != nil {
		s.clientError(w, http.StatusBadRequest, "invalid push payload")
		return
	}

	fullName := strings.TrimSuffix(strings.TrimPrefix(cfg.Repository, "https://github.com/"), ".git")

	if !strings.EqualFold(push.Repository.FullName, fullName) {
		s.clientError(w, http.StatusForbidden, "repository is not allowed")
		return
	}

	if push.Deleted || strings.HasPrefix(push.Ref, "refs/tags/") {
		s.apiResponse(w, http.StatusAccepted, nil, "push ignored", nil)
		return
	}

	branch, ok := strings.CutPrefix(push.Ref, "refs/heads/")

	if !ok || strings.TrimSpace(branch) == "" {
		s.clientError(w, http.StatusBadRequest, "push must include a branch ref")
		return
	}

	_, err = hex.DecodeString(push.After)

	if err != nil || len(push.After) != 40 || push.After == strings.Repeat("0", 40) {
		s.clientError(w, http.StatusBadRequest, "push must include a commit SHA")
		return
	}

	deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))

	if deliveryID == "" {
		s.clientError(w, http.StatusBadRequest, "X-GitHub-Delivery is required")
		return
	}

	run, err := s.app.Models.Runs.CreateRun(r.Context(), repository.CreateRunInput{
		Repository: cfg.Repository,
		Branch:     branch,
		CommitSHA:  push.After,
		DeliveryID: &deliveryID,
	})

	if err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.apiResponse(w, http.StatusAccepted, run, "", nil); err != nil {
		s.serverError(w, err)
	}
}
