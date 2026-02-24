.PHONY: build test lint run migrate-up migrate-down sqlc docker-dev docker-prod deploy datamigrate clean

# Variables
APP_NAME := aistarlight
GO := go
GOFLAGS := -race
DOCKER_COMPOSE := docker compose

# Build targets
build: build-api build-worker build-migrate build-datamigrate

build-api:
	$(GO) build -o bin/api ./cmd/api

build-worker:
	$(GO) build -o bin/worker ./cmd/worker

build-migrate:
	$(GO) build -o bin/migrate ./cmd/migrate

build-datamigrate:
	$(GO) build -o bin/datamigrate ./cmd/datamigrate

# Run targets
run-api:
	$(GO) run ./cmd/api

run-worker:
	$(GO) run ./cmd/worker

# Test
test:
	$(GO) test $(GOFLAGS) ./... -count=1

test-cover:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

test-short:
	$(GO) test -short ./... -count=1

# Lint
lint:
	golangci-lint run ./...

vet:
	$(GO) vet ./...

# Database migrations
migrate-up:
	$(GO) run ./cmd/migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	$(GO) run ./cmd/migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

# sqlc
sqlc:
	sqlc generate

# Docker — local development
docker-dev:
	$(DOCKER_COMPOSE) -f docker/docker-compose.dev.yaml up -d

docker-dev-down:
	$(DOCKER_COMPOSE) -f docker/docker-compose.dev.yaml down

# Docker — production
docker-prod:
	$(DOCKER_COMPOSE) -f docker-compose.prod.yml up -d --build

docker-prod-down:
	$(DOCKER_COMPOSE) -f docker-compose.prod.yml down

docker-logs:
	$(DOCKER_COMPOSE) -f docker-compose.prod.yml logs -f --tail=100

docker-ps:
	$(DOCKER_COMPOSE) -f docker-compose.prod.yml ps

# Deploy
deploy:
	./deploy/deploy.sh

deploy-migrate:
	./deploy/deploy.sh --migrate-only

# Data migration (Python → Go)
datamigrate:
	$(GO) run ./cmd/datamigrate -source "$(SOURCE_DB)" -target "$(DATABASE_URL)"

datamigrate-dry:
	$(GO) run ./cmd/datamigrate -source "$(SOURCE_DB)" -target "$(DATABASE_URL)" --dry-run

datamigrate-verify:
	$(GO) run ./cmd/datamigrate -source "$(SOURCE_DB)" -target "$(DATABASE_URL)" --verify-only

# Cross-compile for Linux (CI/deploy)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o aistarlight-api ./cmd/api
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o aistarlight-worker ./cmd/worker
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o aistarlight-migrate ./cmd/migrate

# Clean
clean:
	rm -rf bin/ coverage.out coverage.html aistarlight-api aistarlight-worker aistarlight-migrate
