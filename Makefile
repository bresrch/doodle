.PHONY: all test test-unit test-integration docker-up docker-down clean deps

all: deps test

deps:
	go mod tidy

test: test-unit

test-unit:
	go test -v -race ./... -count=1

test-integration: docker-up
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	go test -v -race -tags=integration ./... -count=1

docker-up:
	docker compose up -d
	@echo "Waiting for PostgreSQL to initialize..."
	@until docker compose exec -T postgres pg_isready -U doodle -d doodle_test > /dev/null 2>&1; do \
		echo "Waiting for PostgreSQL..."; \
		sleep 1; \
	done
	@echo "PostgreSQL is ready"

docker-down:
	docker compose down -v

docker-logs:
	docker compose logs -f postgres

clean: docker-down
	go clean -testcache

reset: docker-down docker-up

# Development helpers
compile-example:
	@go run ./cmd/example/main.go

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
