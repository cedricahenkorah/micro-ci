package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
)

type apiResponse struct {
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func (s *Server) apiResponse(w http.ResponseWriter, code int, data any, message string, headers http.Header) error {
	status := "success"

	if code >= 400 {
		status = "error"
	}

	response := apiResponse{
		Status:  status,
		Code:    code,
		Message: message,
		Data:    data,
	}

	js, err := json.Marshal(response)

	if err != nil {
		return err
	}

	js = append(js, '\n')

	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	_, writeErr := w.Write(js)

	if writeErr != nil {
		s.app.ErrorLog.Printf("write response: %v", writeErr)
	}

	return nil
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())

	s.app.ErrorLog.Output(2, trace)

	s.apiResponse(w, http.StatusInternalServerError, nil, http.StatusText(http.StatusInternalServerError), nil)
}

func (s *Server) clientError(w http.ResponseWriter, status int, message string) {
	s.apiResponse(w, status, nil, message, nil)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.apiResponse(w, http.StatusNotFound, nil, http.StatusText(http.StatusNotFound), nil)
}
