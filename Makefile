.PHONY: build run test clean install fmt vet tidy docker-build docker-up docker-down docker-logs

BIN := bin
SRC := ./cmd/tido
VERSION ?= dev

build:
	@mkdir -p $(BIN)
	go build -ldflags "-s -w -X main.Version=$(VERSION)" -o $(BIN)/tido $(SRC)

run: build
	./$(BIN)/tido

test:
	go test -race -count=1 ./...

clean:
	rm -rf $(BIN)

install: build
	@mkdir -p $(HOME)/.local/bin
	install -m 755 $(BIN)/tido $(HOME)/.local/bin/tido
	@echo "installed to $(HOME)/.local/bin/tido"

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# 兼容 v2 plugin (`docker compose`) 与 v1 独立 binary (`docker-compose`)
COMPOSE := $(shell command -v docker-compose >/dev/null 2>&1 && echo docker-compose || echo "docker compose")

docker-build:
	$(COMPOSE) build

docker-up:
	@test -f .env || (echo "missing .env (cp .env.example .env 后填 TIDO_TOKEN)" && exit 1)
	$(COMPOSE) up -d

docker-down:
	$(COMPOSE) down

docker-logs:
	$(COMPOSE) logs -f tido
