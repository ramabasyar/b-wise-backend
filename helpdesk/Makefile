.PHONY: build run dev test clean docker-up docker-down

# Build the application
build:
	go build -o bin/server ./cmd/server

# Run the application
run:
	go run ./cmd/server

# Run with hot-reload (requires air)
dev:
	air

# Run tests
test:
	go test ./... -v -cover

# Clean build artifacts
clean:
	rm -rf bin/ tmp/

# Start Docker services
docker-up:
	docker-compose up -d

# Stop Docker services
docker-down:
	docker-compose down
