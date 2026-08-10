.PHONY: all build test lint run docker-build clean

APP_NAME = coord-server
BUILD_DIR = bin
ENTRY_POINT = ./cmd/server

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(ENTRY_POINT)

test:
	go test -v ./...

lint:
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run; \
	else \
		go vet ./...; \
	fi

run: build
	./$(BUILD_DIR)/$(APP_NAME)

docker-build:
	docker build -t goy-co/coord-server:latest -f deploy/Dockerfile .

clean:
	rm -rf $(BUILD_DIR) *.db *.db-wal *.db-shm
