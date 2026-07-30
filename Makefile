.PHONY: help run build test test-mongo test-sql test-models wirelog check-i18n api-bundle api-check api-lock api-surface tidy vet fmt clean docker docker-up db-postgres db-mysql db-mongo

BIN := bin/daycore

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

run: ## Run the server (SQLite by default)
	go run ./cmd/daycore

build: ## Build a static binary into bin/
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN) ./cmd/daycore

test-sql: ## Run the storage conformance suite against real PostgreSQL and MySQL
	@echo "needs a postgres on :5432 and a mysql on :3306 — each case creates and drops its own schema/database"
	@echo "  docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=daycore -e POSTGRES_USER=daycore -e POSTGRES_DB=daycore postgres:16"
	@echo "  docker run -d -p 3306:3306 -e MYSQL_ROOT_PASSWORD=daycore -e MYSQL_DATABASE=daycore mysql:8"
	PG_TEST_DSN='postgres://daycore:daycore@127.0.0.1:5432/daycore?sslmode=disable' \
	MYSQL_TEST_DSN='root:daycore@tcp(127.0.0.1:3306)/daycore?parseTime=true' \
	go test -count=1 -run 'TestConformancePostgres|TestConformanceMySQL|TestRealDialectNamespaces' -v ./internal/storage/sqlstore/

test-models: ## Live tool-calling check against real models (costs money; never in CI)
	@echo "needs LIVE_MODEL_BASE_URL / LIVE_MODEL_API_KEY / LIVE_MODELS"
	@echo "  LIVE_MODELS=deepseek-v4-flash,glm-5.2,grok-4.5 make test-models"
	go test -count=1 -timeout 900s -parallel 3 -run 'TestLiveToolCalling' -v ./internal/ai/

wirelog: ## Logging reverse proxy — see exactly what we send a provider (tools/wirelog)
	go run ./tools/wirelog -listen 127.0.0.1:8899 -upstream $(UPSTREAM) -out /tmp/daycore-wire

test-mongo: ## Run the storage conformance suite against a real MongoDB
	@echo "requires a mongod on 127.0.0.1:27017 — the suite creates and drops its own databases"
	MONGO_TEST_DSN=mongodb://127.0.0.1:27017 go test -count=1 -run TestConformance -v ./internal/storage/mongostore/

test: check-i18n ## Run all tests
	go test ./...

check-i18n: ## Verify zh-CN/en-US i18n key sets stay aligned
	node web/frontend/scripts/check-i18n.mjs

api-bundle: ## Rebuild api/openapi.yaml from the per-tag shards in api/spec/
	go run ./api/spec/bundle

api-check: ## Verify api/openapi.yaml matches the shards (also covered by go test)
	go run ./api/spec/bundle -check

api-lock: ## Freeze the current contract surface at the current APIVersion.APIMinor
	go run ./api/spec/bundle -lock

api-surface: ## Regenerate the route table in docs/API_SURFACE.md from the registry
	go test ./internal/server/ -run TestRouteSurfaceDoc -update

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
