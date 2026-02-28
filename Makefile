.PHONY: all build test lint run dev clean docker help setup migrate infra-bootstrap observability-install

# Variables
BINARY_NAME := dkpbot
GO := go
GOFLAGS := -v
LDFLAGS := -ldflags="-s -w -X main.version=dev"
CONFIG := config.example.yaml

# Default target
all: lint test build

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'

## setup: Install development tools and dependencies
setup:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	$(GO) install github.com/goreleaser/goreleaser/v2@latest
	$(GO) mod download
	@echo "Development tools installed."

## build: Build the binary
build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/dkpbot

## test: Run all tests
test:
	$(GO) test -race -count=1 ./...

## test-cover: Run tests with coverage
test-cover:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	$(GO) fmt ./...
	goimports -w -local github.com/jensholdgaard/discord-dkp-bot .

## run: Run the bot with example config
run: build
	./bin/$(BINARY_NAME) --config $(CONFIG)

## dev: Start local development environment (Postgres via Docker)
dev:
	docker compose -f deploy/docker-compose.dev.yml up -d
	@echo "Waiting for Postgres to be ready..."
	@sleep 3
	@echo "Development environment ready. Postgres on localhost:5432"

## dev-down: Stop local development environment
dev-down:
	docker compose -f deploy/docker-compose.dev.yml down -v

## migrate: Run database migrations
migrate:
	@echo "Applying migrations..."
	@for f in internal/store/postgres/migrations/*.sql; do \
		echo "Applying $$f"; \
		PGPASSWORD=changeme psql -h localhost -U dkpbot -d dkpbot -f "$$f"; \
	done

## docker: Build Docker image
docker:
	docker build -t $(BINARY_NAME):dev .

## release-snapshot: Build a snapshot release (no publish)
release-snapshot:
	goreleaser release --snapshot --clean

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/ coverage.out coverage.html

## tidy: Tidy go modules
tidy:
	$(GO) mod tidy

## infra-bootstrap: Bootstrap Hetzner Cloud Kubernetes cluster via Cluster API
infra-bootstrap:
	bash deploy/infrastructure/bootstrap.sh

## observability-install: Install observability stack (Prometheus, Grafana, Loki, Tempo)
observability-install:
	bash deploy/observability/install.sh
