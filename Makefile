.PHONY: build build-baseapp build-seedadmin build-agent run dev tidy fmt vet test \
        migrate-up migrate-down migrate-status seed-admin clean ui-build ui-dev

GO        := /usr/local/go/bin/go
BIN_DIR   := bin
LDFLAGS   := -ldflags="-s -w"

ifneq (,$(wildcard .env))
include .env
export
endif

build: build-baseapp build-seedadmin build-agent

build-baseapp:
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/baseapp ./cmd/baseapp

build-seedadmin:
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/seedadmin ./cmd/seedadmin

build-agent:
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/node-agent ./cmd/agent

# Static-linked Linux build of the agent, suitable for distributing to nodes.
build-agent-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build -trimpath $(LDFLAGS) -o $(BIN_DIR)/node-agent-linux-amd64 ./cmd/agent

run: build-baseapp
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

ui-build:
	cd web && npm install --silent && npm run build

ui-dev:
	cd web && npm run dev

clean:
	rm -rf $(BIN_DIR)
