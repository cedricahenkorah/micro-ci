package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
)

func TestHealthcheckReturnsSingleJSONResponse(t *testing.T) {
	app := &config.Application{}
	app.Config.Env = "development"
	server := &Server{app: app}
	response := httptest.NewRecorder()

	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body struct {
		Status string            `json:"status"`
		Code   int               `json:"code"`
		Data   map[string]string `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response must be a single JSON document: %v", err)
	}
	if body.Status != "success" || body.Code != http.StatusOK ||
		body.Data["status"] != "available" || body.Data["environment"] != "development" {
		t.Fatalf("unexpected health response: %+v", body)
	}
}

func TestAPIResponseLogsWriteFailureWithoutRetry(t *testing.T) {
	var logs bytes.Buffer
	server := &Server{app: &config.Application{ErrorLog: log.New(&logs, "", 0)}}
	writeErr := errors.New("client connection closed")
	writer := &failingResponseWriter{header: make(http.Header), err: writeErr}

	if err := server.apiResponse(writer, http.StatusOK, nil, "", nil); err != nil {
		t.Fatalf("write failure must be logged after committing headers, got %v", err)
	}
	if writer.writes != 1 {
		t.Errorf("body writes = %d, want 1", writer.writes)
	}
	if !strings.Contains(logs.String(), writeErr.Error()) {
		t.Errorf("log does not contain the write error: %q", logs.String())
	}
}

type failingResponseWriter struct {
	header http.Header
	err    error
	writes int
}

func (w *failingResponseWriter) Header() http.Header { return w.header }
func (w *failingResponseWriter) WriteHeader(int)     {}
func (w *failingResponseWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}
