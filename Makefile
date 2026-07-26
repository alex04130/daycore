.PHONY: help run build test check-i18n tidy vet fmt clean docker docker-up db-postgres db-mysql db-mongo

BIN := bin/daycore

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

run: ## Run the server (SQLite by default)
	go run ./cmd/daycore

build: ## Build a static binary into bin/
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN) ./cmd/daycore

test: check-i18n ## Run all tests
	go test ./...

check-i18n: ## Verify zh-CN/en-US i18n key sets stay aligned
	node web/frontend/scripts/check-i18n.mjs

vet: ## go vet
	go vet ./...

fmt: ## gofmt all packages
	gofmt -w .

tidy: ## go mod tidy
	go mod tidy

clean: ## Remove build artifacts + local sqlite db
	rm -rf bin daycore.db daycore.db-shm daycore.db-wal

docker: ## Build the Docker image
	docker build -f deploy/Dockerfile -t daycore:latest .

docker-up: ## Run via docker-compose (SQLite). Set JWT_SECRET/COOKIE_SECRET first.
	docker compose -f deploy/docker-compose.yml up app

db-postgres: ## Start a local Postgres for development
	docker compose -f deploy/docker-compose.yml --profile postgres up -d postgres

db-mysql: ## Start a local MySQL for development
	docker compose -f deploy/docker-compose.yml --profile mysql up -d mysql

db-mongo: ## Start a local MongoDB for development
	docker compose -f deploy/docker-compose.yml --profile mongo up -d mongo
