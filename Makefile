.PHONY: build run test clean install fmt vet tidy

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
