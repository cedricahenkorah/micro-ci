package api

import (
	"net/http"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

type Server struct {
	app *config.Application
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: s.app.Config.CORSAllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type", "Last-Event-ID"},
		MaxAge:         300,
	}))

	r.NotFound(s.notFound)

	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Get("/health", s.healthcheck)
			r.Post("/webhooks/github", s.githubWebhook)
			r.Post("/runs", s.createRun)
			r.Get("/runs", s.listRuns)
			r.Get("/runs/{id}", s.getRun)
			r.Get("/runs/{id}/events", s.streamEvents)
			r.Post("/runs/{id}/cancel", s.cancelRun)
			r.Post("/runs/{id}/retry", s.retryRun)
		})

	})

	return r
}
