.DEFAULT_GOAL := help

GO ?= go
PORT ?= 4000
ARGS ?=

.PHONY: help build api worker test vet check fmt health

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
