package api

import (
	"net/http"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	app *config.Application
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.NotFound(s.notFound)

	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Get("/health", s.healthcheck)
			r.Post("/webhooks/github", s.githubWebhook)
			r.Post("/runs", s.createRun)
		})

	})

	return r
}
