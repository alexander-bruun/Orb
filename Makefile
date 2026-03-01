DATABASE_URL       ?= postgres://orb:orb@localhost:5432/orb?sslmode=disable
KV_SENTINEL_ADDRS  ?= localhost:26379
KV_SENTINEL_MASTER ?= mymaster
STORE_BACKEND      ?= local
STORE_ROOT         ?= ./data/audio
HTTP_PORT          ?= 8080
DIR                ?= ./music
USER_ID            ?=

.PHONY: dev-db dev-api dev-ui dev-ingest \
        migrate-up migrate-down migrate-diff migrate-status \
        generate test lint

dev-db:
	docker compose -f docker-compose.dev.yml up -d

dev-api:
	cd services/api && \
	DATABASE_URL=$(DATABASE_URL) KV_SENTINEL_ADDRS=$(KV_SENTINEL_ADDRS) \
	KV_SENTINEL_MASTER=$(KV_SENTINEL_MASTER) STORE_BACKEND=$(STORE_BACKEND) \
	STORE_ROOT=$(STORE_ROOT) JWT_SECRET=dev-secret-change-in-prod \
	HTTP_PORT=$(HTTP_PORT) go run ./cmd/main.go

dev-ui:
	cd web/ui && bun run dev

dev-ingest:
	cd cmd/ingest && \
	DATABASE_URL=$(DATABASE_URL) STORE_BACKEND=$(STORE_BACKEND) \
	STORE_ROOT=$(STORE_ROOT) \
	go run . --dir $(DIR) --user-id $(USER_ID) --recursive

migrate-up:     ; atlas migrate apply --env local
migrate-down:   ; atlas migrate down --env local --amount 1
migrate-status: ; atlas migrate status --env local
migrate-diff:
	@read -p "Migration name: " name; \
	atlas migrate diff --env local --name $$name

test:
	go test ./...

lint:
	golangci-lint run ./...
