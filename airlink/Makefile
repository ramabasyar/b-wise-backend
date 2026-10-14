.PHONY: run build clean test deps

APP_NAME := airlink
MAIN_PATH := cmd/server/main.go

all: deps run

deps:
	go mod download
	go mod tidy

run:
	go run $(MAIN_PATH)

build:
	CGO_ENABLED=0 go build -o bin/$(APP_NAME) $(MAIN_PATH)

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/$(APP_NAME)-linux-amd64 $(MAIN_PATH)

clean:
	rm -rf bin/
	go clean
