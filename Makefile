.PHONY: build test test-store lint check dev

build:
	go build -o bin/auth-gate ./cmd/auth-gate

test:
	go test ./...

test-store:
	docker compose up -d dynamodb
	DYNAMO_ENDPOINT=http://localhost:8000 go test ./...

lint:
	golangci-lint run

check:
	go vet ./...
	golangci-lint run
	go test ./...

dev:
	@echo "dev: not implemented yet (Task 11)"
