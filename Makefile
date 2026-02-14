.PHONY: build test clean run lint

BINARY_NAME=nexus
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/nexus

test:
	go test -v -race -count=1 ./...

test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html

run: build
	$(BUILD_DIR)/$(BINARY_NAME)

lint:
	go vet ./...
