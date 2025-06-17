# Makefile for building, testing and running the agent system
.PHONY: build test lint run run-redis stop demo

# Build the Go application
build:
go build -o bin/agentctl ./cmd/agentctl

# Run unit tests
test:
go test ./...

# Static analysis
lint:
go vet ./...
golangci-lint run

# Run the application after building
run: build
./bin/agentctl --config configs/default.yaml

# Start Redis via Docker Compose
run-redis:
docker-compose up -d redis

# Stop all Docker Compose services
stop:
docker-compose down

# Run the demo scenario
# Starts Redis and executes the application in demo mode

demo: run-redis
./bin/agentctl --config configs/default.yaml --demo
