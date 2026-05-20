.PHONY: build run dev tidy fmt vet test migrate-up migrate-down migrate-status clean

GO        := /usr/local/go/bin/go
BIN_DIR   := bin
BIN_NAME  := baseapp
LDFLAGS   := -ldflags="-s -w"

# Load .env if present (so DATABASE_URL etc. are available to migrate)
ifneq (,$(wildcard .env))
include .env
export
endif

build:
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(BIN_NAME) ./cmd/baseapp

run: build
	./$(BIN_DIR)/$(BIN_NAME)

dev:
	$(GO) run ./cmd/baseapp

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

migrate-up:
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$DATABASE_URL" down 1

migrate-status:
	migrate -path migrations -database "$$DATABASE_URL" version

clean:
	rm -rf $(BIN_DIR)
