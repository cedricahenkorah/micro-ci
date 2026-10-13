package config

import (
	"flag"
	"fmt"
	"log"
	"os"
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

	return cfg, nil
}

func parseTimeDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)

	if err != nil {
		log.Printf("invalid query timeout %q; using default %s", value, fallback)
		return fallback
	}

	return duration
}
