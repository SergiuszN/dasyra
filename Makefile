.PHONY: build test lint run up down

build: ## Build the bot
	go build -o bin/dasyra ./cmd/dasyra

test: ## Unit tests with the race detector
	go test -race ./...

lint:
	gofmt -l . && go vet ./...

run: build ## Local run with long polling, env from .env, SearXNG from `make up`
	set -a; . ./.env; set +a; SEARXNG_URL=$${SEARXNG_URL:-http://localhost:8888} ./bin/dasyra poll

up: ## Local SearXNG on :8888
	docker compose up -d

down:
	docker compose down --remove-orphans
