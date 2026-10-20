package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port int
	Env  string
	Db   struct {
		Dsn          string
		QueryTimeout time.Duration
	}
	ShutdownTimeout time.Duration
	Worker          struct {
		Concurrency       int
		PollInterval      time.Duration
		LeaseDuration     time.Duration
		HeartbeatInterval time.Duration
		WorkspaceRoot     string
	}
}

func Load(args []string) (Config, error) {
	var cfg Config

	err := godotenv.Load()

	if err != nil && !os.IsNotExist(err) {
		return cfg, fmt.Errorf("load .env: %w", err)
	}

	flags := flag.NewFlagSet("micro-ci", flag.ContinueOnError)

	flags.IntVar(&cfg.Port, "port", 4000, "HTTP network port")
	flags.StringVar(&cfg.Db.Dsn, "dsn", "", "PostgreSQL DSN; defaults to DATABASE_URL")
	flags.DurationVar(&cfg.Db.QueryTimeout, "queryTimeout", parseTimeDuration(os.Getenv("DATABASE_QUERY_TIMEOUT"), 3*time.Second), "Database query timeout")
	flags.DurationVar(&cfg.ShutdownTimeout, "shutdownTimeout", parseTimeDuration(os.Getenv("SHUTDOWN_TIMEOUT"), 3*time.Second), "Graceful shutdown timeout")
	flags.StringVar(&cfg.Env, "env", os.Getenv("ENV"), "Environment(developmentundefined: parseTimeDuration|staging|production)")
	flags.DurationVar(
		&cfg.Worker.PollInterval,
		"pollInterval",
		parseTimeDuration(os.Getenv("WORKER_POLL_INTERVAL"), time.Second),
		"Wait between claims when the queue is empty",
	)
	flags.DurationVar(
		&cfg.Worker.LeaseDuration,
		"leaseDuration",
		parseTimeDuration(os.Getenv("WORKER_LEASE_DURATION"), 30*time.Second),
		"Worker claim lease duration",
	)
	flags.DurationVar(
		&cfg.Worker.HeartbeatInterval,
		"heartbeatInterval",
		parseTimeDuration(os.Getenv("WORKER_HEARTBEAT_INTERVAL"), 5*time.Second),
		"Interval between lease renewals",
	)

	concurrency := 2

	if value := os.Getenv("WORKER_CONCURRENCY"); value != "" {
		parsed, err := strconv.Atoi(value)

		if err != nil {
			return cfg, fmt.Errorf("invalid WORKER_CONCURRENCY: %w", err)
		}

		concurrency = parsed
	}

	flags.IntVar(
		&cfg.Worker.Concurrency,
		"workerConcurrency",
		concurrency,
		"Maximum concurrent runs per worker process",
	)

	workspaceRoot := os.Getenv("WORKER_WORKSPACE_ROOT")

	if workspaceRoot == "" {
		workspaceRoot = "/tmp/micro-ci/workspaces"
	}

	flags.StringVar(
		&cfg.Worker.WorkspaceRoot,
		"workspaceRoot",
		workspaceRoot,
		"Root directory for attempt workspaces",
	)

	parseFlagErr := flags.Parse(args)

	if parseFlagErr != nil {
		return cfg, parseFlagErr
	}

	if flags.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected positional arguments after mode")
	}

	if cfg.Db.Dsn == "" {
		cfg.Db.Dsn = os.Getenv("DATABASE_URL")
	}

	if strings.TrimSpace(cfg.Db.Dsn) == "" {
		return cfg, fmt.Errorf("provide -dsn or set DATABASE_URL")
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("port must be between 1 and 65535")
	}

	if cfg.Db.QueryTimeout <= 0 {
		return cfg, fmt.Errorf("query-timeout must be positive")
	}

	if cfg.ShutdownTimeout <= 0 {
		return cfg, fmt.Errorf("shutdown-timeout must be positive")
	}

	if cfg.Worker.PollInterval <= 0 {
		return cfg, fmt.Errorf("pollInterval must be positive")
	}

	if cfg.Worker.LeaseDuration <= 0 {
		return cfg, fmt.Errorf("leaseDuration must be positive")
	}

	if cfg.Worker.HeartbeatInterval <= 0 {
		return cfg, fmt.Errorf("heartbeatInterval must be positive")
	}

	if cfg.Worker.HeartbeatInterval >= cfg.Worker.LeaseDuration {
		return cfg, fmt.Errorf("heartbeatInterval must be shorter than leaseDuration")
	}

	if cfg.Worker.Concurrency <= 0 {
		return cfg, fmt.Errorf("workerConcurrency must be positive")
	}

	root, workspaceRootErr := filepath.Abs(cfg.Worker.WorkspaceRoot)

	if workspaceRootErr != nil {
		return cfg, fmt.Errorf("resolve workspace root: %w", workspaceRootErr)
	}

	cfg.Worker.WorkspaceRoot = root

	return cfg, nil
}

func parseTimeDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)

	if err != nil {
		log.Printf("invalid duration timeout %q; using default %s", value, fallback)
		return fallback
	}

	return duration
}
