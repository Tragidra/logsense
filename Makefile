.PHONY: help build test test-race lint dev-up dev-down pre-commit clean web-dev web-build web-install migrate seed

GO       ?= go
GOLANGCI ?= golangci-lint
BINARY   := logsense
COMPOSE  := docker compose -f docker-compose.dev.yml

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X github.com/Tragidra/logsense/pkg/version.Version=$(VERSION) -X github.com/Tragidra/logsense/pkg/version.Commit=$(COMMIT)"

help:
	@echo "Targets:"
	@echo "  build         - compile logsense binary"
	@echo "  test          - run unit tests"
	@echo "  test-race     - run tests with race detector"
	@echo "  lint          - run golangci-lint"
	@echo "  dev-up        - docker compose up (postgres, for integration tests)"
	@echo "  dev-down      - docker compose down"
	@echo "  migrate       - apply migrations to ./logsense.db (SQLite default or PG)"
	@echo "  pre-commit    - fmt + vet + test + lint"
	@echo "  web-install   - npm install in web/"
	@echo "  web-dev       - vite dev server on :5173"
	@echo "  web-build     - production frontend build (output embedded into binary)"
	@echo "  seed          - regenerate scripts/seed-data/ fixtures"
	@echo "  clean         - remove build artifacts"

build:
	$(GO) build $(LDFLAGS) -o bin/$(BINARY) ./cmd/logsense

test:
	$(GO) test ./... -count=1

test-race:
	$(GO) test ./... -race -count=1

lint:
	$(GOLANGCI) run

dev-up:
	$(COMPOSE) up -d

dev-down:
	$(COMPOSE) down

migrate: build
	./bin/$(BINARY) migrate

pre-commit:
	$(GO) fmt ./...
	$(GO) vet ./...
	$(GO) test ./... -count=1
	$(GOLANGCI) run

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

seed:
	python3 scripts/seed-data/generate.py

clean:
	rm -rf bin/ web/dist/ data/
