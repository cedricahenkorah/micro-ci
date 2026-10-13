package api

import "net/http"

func (s *Server) healthcheck(w http.ResponseWriter, r *http.Request) {
	data := map[string]string{
		"status":      "available",
		"environment": s.app.Config.Env,
	}

	err := s.apiResponse(w, http.StatusOK, data, "", nil)

	if err != nil {
		s.serverError(w, err)
	}
}
