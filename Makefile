.PHONY: all help build test lint run docker-build clean

APP_NAME = coord-server
BUILD_DIR = bin
ENTRY_POINT = ./cmd/server

all: help

help: ## Exibe os comandos disponíveis
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Compila o binário do coord-server
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(ENTRY_POINT)

test: ## Executa os testes unitários
	go test -v ./...

lint: ## Executa o linter (golangci-lint ou go vet)
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run; \
	else \
		go vet ./...; \
	fi

run: build ## Compila e executa o servidor localmente
	./$(BUILD_DIR)/$(APP_NAME)

docker-build: ## Constrói a imagem Docker
	docker build -t goy-co/coord-server:latest -f deploy/Dockerfile .

clean: ## Remove binários e bases de dados temporárias
	rm -rf $(BUILD_DIR) *.db *.db-wal *.db-shm
