.PHONY: build test test-unit tidy run compose-up compose-down

# Binary build
build:
	go build -o bin/gateway-api ./cmd/gateway-api

# Run local unit tests
test:
	go test -v -race ./...

test-unit:
	go test -v ./internal/auth/... ./internal/ratelimit/... ./internal/provider/...

# Dependency management
tidy:
	go mod tidy

# Run gateway service locally
run:
	go run ./cmd/gateway-api

# Docker Compose workflows
compose-up:
	docker compose -f deploy/compose/docker-compose.yml up -d

compose-down:
	docker compose -f deploy/compose/docker-compose.yml down -v
