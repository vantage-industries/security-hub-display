BINARY_NAME := security-hub-display
BIN_DIR := bin

.PHONY: all build build-linux-arm64 build-linux-armv7 build-linux-amd64 test clean run-terminal run-mock run-web

all: test build

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/security-hub-display
	@echo "Built $(BIN_DIR)/$(BINARY_NAME)"

build-linux-arm64:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/security-hub-display
	@echo "Built $(BIN_DIR)/$(BINARY_NAME)-linux-arm64"

build-linux-armv7:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME)-linux-armv7 ./cmd/security-hub-display
	@echo "Built $(BIN_DIR)/$(BINARY_NAME)-linux-armv7"

build-linux-amd64:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/security-hub-display
	@echo "Built $(BIN_DIR)/$(BINARY_NAME)-linux-amd64"

build-hello: build-hello-arm64 build-hello-armv7

build-hello-arm64:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/hello-display-arm64 ./cmd/hello-display
	@echo "Built $(BIN_DIR)/hello-display-arm64"

build-hello-armv7:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/hello-display-armv7 ./cmd/hello-display
	@echo "Built $(BIN_DIR)/hello-display-armv7"

build-all: build-linux-arm64 build-linux-armv7 build-linux-amd64 build-hello

test:
	CGO_ENABLED=0 go test -v ./...

run-mock: build
	./$(BIN_DIR)/$(BINARY_NAME) --mode=terminal --mock

run-web: build
	./$(BIN_DIR)/$(BINARY_NAME) --mode=web --mock --web-port=8085

run-stdin: build
	./$(BIN_DIR)/$(BINARY_NAME) --mode=terminal --stdin

clean:
	rm -rf $(BIN_DIR)
