package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cedricahenkorah/micro-ci/backend/internal/api"
	"github.com/cedricahenkorah/micro-ci/backend/internal/config"
	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
	"github.com/cedricahenkorah/micro-ci/backend/internal/worker"
)

func main() {
	err := run()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: micro-ci <api|worker> [flags]")
	}

	mode := os.Args[1]

	if mode != "api" && mode != "worker" {
		return fmt.Errorf("unknown mode %q: use api or worker", mode)
	}

	cfg, configLoadErr := config.Load(os.Args[2:])

	if errors.Is(configLoadErr, flag.ErrHelp) {
		return nil
	}

	if configLoadErr != nil {
		return fmt.Errorf("load configuration: %w", configLoadErr)
	}

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)

	defer stop()

	db, dbErr := config.OpenDB(cfg.Db.Dsn, cfg.Db.QueryTimeout, ctx)

	if dbErr != nil {
		return fmt.Errorf("connect to database: %w", dbErr)
	}

	defer db.Close()

	app := &config.Application{
		ErrorLog: errorLog,
		InfoLog:  infoLog,
		Models:   repository.New(db, cfg.Db.QueryTimeout),
		Config:   cfg,
	}

	switch os.Args[1] {
	case "api":
		return api.Run(ctx, app)
	case "worker":
		return worker.Run(ctx, app)
	}

	return nil
}
