.PHONY: all test test-unit test-integration docker-up docker-down clean deps
.PHONY: unit-up unit-down unit-logs unit-reset test-unit-db

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

# Unit test database with seeded data
test-unit-db: unit-up
	@echo "Running integration tests against unit database (with seed data)..."
	DOODLE_TEST_DB="postgres://doodle:doodle@localhost:5440/doodle_unit?sslmode=disable" \
		go test -v -race -tags=integration ./... -count=1

unit-up:
	docker compose -f docker-compose.unit.yml up -d
	@echo "Waiting for PostgreSQL to initialize with seed data..."
	@until docker compose -f docker-compose.unit.yml exec -T postgres pg_isready -U doodle -d doodle_unit > /dev/null 2>&1; do \
		echo "Waiting for PostgreSQL..."; \
		sleep 1; \
	done
	@echo "PostgreSQL is ready with init + seed data"

unit-down:
	docker compose -f docker-compose.unit.yml down -v

unit-logs:
	docker compose -f docker-compose.unit.yml logs -f postgres

unit-reset: unit-down unit-up

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

clean: docker-down unit-down
	go clean -testcache

reset: docker-down docker-up

# Development helpers
compile-example:
	@go run ./cmd/example/main.go

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
