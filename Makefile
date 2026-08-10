.PHONY: help run build build-lite cross test test-mongo test-sql test-models wirelog check-i18n api-bundle api-check api-lock api-surface config-doc tidy vet fmt clean docker docker-up db-postgres db-mysql db-mongo

BIN := bin/daycore
VERSION := $(shell sed -n 's/.*Version = "\(.*\)".*/\1/p' internal/version/version.go)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

run: ## Run the server (SQLite by default)
	go run ./cmd/daycore

build: ## Build a static binary into bin/
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN) ./cmd/daycore

build-lite: ## Build the lite binary + its data pack into dist/
	@# The lite binary embeds nothing, so the pack is not optional packaging —
	@# it is half the artifact. Shipping one without the other produces a binary
	@# that cannot start, which is the failure this whole build target exists to
	@# make impossible to do by accident.
	CGO_ENABLED=0 go build -tags lite -ldflags="-s -w" -o dist/daycore-lite ./cmd/daycore
	rm -rf dist/data && mkdir -p dist/data
	cp -r internal/resources/data/prompts internal/resources/data/seed dist/data/
	tar czf dist/daycore-data-$(VERSION).tar.gz -C dist/data .
	@echo "dist/daycore-lite + dist/daycore-data-$(VERSION).tar.gz"
	@echo "unpack the pack next to the binary, or point DAYCORE_DATA_DIR at it"

# The two DSNs the SQL conformance suite connects with.
#
# `?=` so an existing environment variable WINS. It matters more than it looks:
# a developer machine that already runs a Postgres or a MySQL for something else
# has those ports taken, and the daycore containers land on 3307 or 5433. Before
# this, `make test-sql` overrode whatever was exported and produced 46 identical
# authentication failures — which reads exactly like a broken suite.
PG_TEST_DSN ?= postgres://daycore:daycore@127.0.0.1:5432/daycore?sslmode=disable
MYSQL_TEST_DSN ?= root:daycore@tcp(127.0.0.1:3306)/daycore?parseTime=true

cross: ## Compile and vet for every released platform (catches build-tagged files this machine never builds)
	@echo "the restart is per-platform (syscall.Exec on unix, spawn on windows), so these files"
	@echo "are never compiled by a plain \`go build\` here — see cmd/daycore/restart_*.go"
	GOOS=windows GOARCH=amd64 go build -o /dev/null ./...
	GOOS=darwin  GOARCH=arm64 go build -o /dev/null ./...
	GOOS=darwin  GOARCH=amd64 go build -o /dev/null ./...
	GOOS=linux   GOARCH=arm64 go build -o /dev/null ./...
	GOOS=windows go vet ./...
	GOOS=darwin  go vet ./...

test-sql: ## Run the storage conformance suite against real PostgreSQL and MySQL
	@echo "each case creates and drops its own schema/database; export PG_TEST_DSN / MYSQL_TEST_DSN to point elsewhere"
	@echo "  docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=daycore -e POSTGRES_USER=daycore -e POSTGRES_DB=daycore postgres:16"
	@echo "  docker run -d -p 3306:3306 -e MYSQL_ROOT_PASSWORD=daycore -e MYSQL_DATABASE=daycore mysql:8"
	@echo "  pg=$(PG_TEST_DSN)"
	PG_TEST_DSN='$(PG_TEST_DSN)' \
	MYSQL_TEST_DSN='$(MYSQL_TEST_DSN)' \
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

config-doc: ## Regenerate the layering tables in docs/CONFIG.md from internal/config
	go test ./internal/config/ -run TestConfigDocIsCurrent -update

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
