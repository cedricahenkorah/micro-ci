.DEFAULT_GOAL := help

GO ?= go
PORT ?= 4000
ARGS ?=

# Reuse the existing dotenv dependency without treating .env as shell code.
# Existing environment variables take precedence over values in .env.
MIGRATE_RUN = $(GO) run github.com/joho/godotenv/cmd/godotenv \
	sh -c 'exec migrate -path migrations -database "$${DATABASE_URL:?DATABASE_URL is required}" "$$@"' --

.PHONY: help build api worker test vet check fmt health migrate-up migrate-down migrate-version migrate-new

help: ## Show available shortcuts
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build bin/micro-ci
	$(GO) build -o bin/micro-ci ./cmd

# Run from backend/ so the application loads backend/.env.
# Example: make api PORT=4001 ARGS='-env development'
api: build ## Build and run the API (PORT=4000; optional ARGS)
	./bin/micro-ci api -port $(PORT) $(ARGS)

worker: build ## Build and run the worker (optional ARGS)
	./bin/micro-ci worker $(ARGS)

test: ## Run all Go tests
	$(GO) test ./...

vet: ## Run Go static checks
	$(GO) vet ./...

check: test vet ## Run tests and static checks

fmt: ## Format Go source files
	$(GO) fmt ./...

health: ## Check the running API's health endpoint (PORT=4000)
	curl --fail --show-error --silent --include --max-time 5 http://localhost:$(PORT)/api/v1/health

migrate-up: ## Apply all pending migrations using backend/.env
	@$(MIGRATE_RUN) up

migrate-down: ## Roll back one migration (may delete data)
	@$(MIGRATE_RUN) down 1

migrate-version: ## Show the current database migration version
	@$(MIGRATE_RUN) version

migrate-new: export MIGRATION_NAME = $(NAME)
migrate-new: ## Create empty SQL migration files: NAME=add_run_priority
	@case "$$MIGRATION_NAME" in \
		''|[!a-z]*|*[!a-z0-9_]*) \
			printf '%s\n' 'Set NAME to a lowercase name starting with a letter (letters, digits, underscores).' >&2; \
			exit 1 ;; \
	esac
	@migrate create -ext sql -dir migrations -seq "$$MIGRATION_NAME"
