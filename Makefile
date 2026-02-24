.PHONY: build test lint run migrate-up migrate-down sqlc docker-dev docker-prod clean

# Variables
APP_NAME := aistarlight
GO := go
GOFLAGS := -race
MIGRATE := migrate
SQLC := sqlc
DOCKER_COMPOSE := docker compose

# Build targets
build: build-api build-worker build-migrate

build-api:
	$(GO) build -o bin/api ./cmd/api

build-worker:
	$(GO) build -o bin/worker ./cmd/worker

build-migrate:
	$(GO) build -o bin/migrate ./cmd/migrate

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
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	$(MIGRATE) -path migrations -database "$(DATABASE_URL)" down 1

migrate-create:
	$(MIGRATE) create -ext sql -dir migrations -seq $(name)

# sqlc
sqlc:
	$(SQLC) generate

# Docker
docker-dev:
	$(DOCKER_COMPOSE) -f docker/docker-compose.dev.yaml up -d

docker-dev-down:
	$(DOCKER_COMPOSE) -f docker/docker-compose.dev.yaml down

docker-prod:
	$(DOCKER_COMPOSE) -f docker-compose.prod.yml up -d --build

docker-prod-down:
	$(DOCKER_COMPOSE) -f docker-compose.prod.yml down

# Clean
clean:
	rm -rf bin/ coverage.out coverage.html
