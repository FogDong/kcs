.PHONY: build install clean test

BINARY_NAME=kcs
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

install:
	go install .

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

test-cover:
	go test -cover ./...

# Build for multiple platforms
build-all:
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
