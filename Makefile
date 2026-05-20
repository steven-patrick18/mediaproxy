.PHONY: build run dev tidy fmt vet test migrate-up migrate-down migrate-status seed-admin clean

GO        := /usr/local/go/bin/go
BIN_DIR   := bin
LDFLAGS   := -ldflags="-s -w"

ifneq (,$(wildcard .env))
include .env
export
endif

build:
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/baseapp ./cmd/baseapp
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/seedadmin ./cmd/seedadmin

run: build
	./$(BIN_DIR)/baseapp

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

# Usage: make seed-admin EMAIL=admin@example.com PASS=longpassword
seed-admin:
	@test -n "$(EMAIL)" || (echo "EMAIL=... required"; exit 1)
	@test -n "$(PASS)"  || (echo "PASS=... required"; exit 1)
	$(GO) run ./cmd/seedadmin "$(EMAIL)" "$(PASS)"

clean:
	rm -rf $(BIN_DIR)
