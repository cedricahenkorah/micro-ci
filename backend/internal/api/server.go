package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
)

func Run(ctx context.Context, app *config.Application) error {
	log.Println("API started")

	server := &Server{app: app}

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.Config.Port),
		ErrorLog:     app.ErrorLog,
		Handler:      server.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	app.Wg.Add(1)

	go func() {
		defer app.Wg.Done()
		recoverExpired(runCtx, app)
	}()

	shutdown := make(chan error, 1)

	go func() {
		<-runCtx.Done()

		app.InfoLog.Println("shuting down the API")

		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), app.Config.ShutdownTimeout)

		defer cancelShutdown()

		err := httpServer.Shutdown(shutdownCtx)

		if err != nil {
			_ = httpServer.Close()
		}

		app.InfoLog.Println("Completing the background tasks", map[string]string{
			"addr": httpServer.Addr,
		})

		app.Wg.Wait()
		shutdown <- err
	}()

	app.InfoLog.Printf("API %s is starting on %s", app.Config.Env, httpServer.Addr)
	serveErr := httpServer.ListenAndServe()

	cancelRun()

	shutdownErr := <-shutdown

	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}

	if shutdownErr != nil {
		return fmt.Errorf("shut down API: %w", shutdownErr)
	}

	app.InfoLog.Printf("API %s is stopping on %s", app.Config.Env, httpServer.Addr)

	return nil
}
