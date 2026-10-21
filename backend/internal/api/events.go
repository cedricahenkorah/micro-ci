package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/domain"
	"github.com/go-chi/chi/v5"
)

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err != nil || runID <= 0 {
		s.clientError(w, http.StatusBadRequest, "id must be a positive run ID")
		return
	}

	var afterSequence int64

	value := strings.TrimSpace(r.Header.Get("Last-Event-ID"))

	if value != "" {
		afterSequence, err = strconv.ParseInt(value, 10, 64)

		if err != nil || afterSequence < 0 {
			s.clientError(w, http.StatusBadRequest, "Last-Event-ID must be a non-negative integer")
			return
		}
	}

	_, err = s.app.Models.Runs.GetRun(r.Context(), runID)

	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, r)
		return
	}

	if err != nil {
		s.serverError(w, err)
		return
	}

	const batchSize = 100

	events, err := s.app.Models.Events.ReadEvents(r.Context(), runID, afterSequence, batchSize)

	if err != nil {
		s.serverError(w, err)
		return
	}

	controller := http.NewResponseController(w)

	err = controller.SetWriteDeadline(time.Time{})

	if err != nil {
		s.serverError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	if err := controller.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if r.Context().Err() != nil {
			return
		}

		if len(events) > 0 {
			err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if err != nil {
				return
			}

			for _, event := range events {
				data, err := json.Marshal(event)

				if err != nil {
					s.app.ErrorLog.Printf("encode stream event: run=%d error=%v", runID, err)
					return
				}

				_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, data)

				if err != nil {
					return
				}

				afterSequence = event.Sequence
			}

			err = controller.Flush()

			if err != nil {
				return
			}

			err = controller.SetWriteDeadline(time.Time{})

			if err != nil {
				return
			}
		}

		if len(events) < batchSize {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}

		events, err = s.app.Models.Events.ReadEvents(r.Context(), runID, afterSequence, batchSize)

		if err != nil {
			if r.Context().Err() == nil {
				s.app.ErrorLog.Printf("read stream events: run=%d error=%v", runID, err)
			}
			return
		}
	}
}
