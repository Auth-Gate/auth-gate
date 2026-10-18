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
	docker compose up -d && ROLE=dev TABLE_NAME=authgate-dev DYNAMO_ENDPOINT=http://localhost:8000 SIGNER=local ADMIN_KEY=dev-admin PORT=8080 BASE_URL=http://localhost:8080 go run ./cmd/auth-gate
