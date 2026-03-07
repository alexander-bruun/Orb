DATABASE_URL       ?= postgres://orb:orb@localhost:5432/orb?sslmode=disable
KV_SENTINEL_ADDRS  ?= localhost:26379
KV_SENTINEL_MASTER ?= mymaster
STORE_BACKEND      ?= local
STORE_ROOT         ?= ./data/audio
HTTP_PORT          ?= 8080
DIR                ?= ./music
USER_ID            ?=

.PHONY: dev-db dev-api dev-ui \
	migrate-up migrate-down migrate-diff migrate-status \
	generate test lint \
	cap-build cap-sync cap-ios cap-android
	cap-ios-build cap-android-build docker-local-up

dev-db:
	docker compose -f docker-compose.dev.yml up -d

dev-api:
	cd services && \
	DATABASE_URL=$(DATABASE_URL) KV_SENTINEL_ADDRS=$(KV_SENTINEL_ADDRS) \
	KV_SENTINEL_MASTER=$(KV_SENTINEL_MASTER) STORE_BACKEND=$(STORE_BACKEND) \
	STORE_ROOT=$(STORE_ROOT) JWT_SECRET=dev-secret-change-in-prod \
	HTTP_PORT=$(HTTP_PORT) go run ./cmd/main.go

dev-ui:
	cd web && bun run dev

cap-build:
	cd web && bun run build

cap-sync: cap-build
	cd web && bunx cap sync

cap-ios: cap-sync
	cd web && bunx cap open ios

cap-android: cap-sync
	cd web && bunx cap open android

cap-ios-build: cap-sync
	cd web && bunx cap build ios

cap-android-build: cap-sync
	cd web && bunx cap build android

# Run docker-compose local build & up (matches developer local command)
docker-local-up:
	sudo docker-compose -f docker-compose.local.yml build && sudo docker-compose -f docker-compose.local.yml up --remove-orphans

# Frontend / Tauri targets used by CI
.PHONY: web-install web-build tauri-build build-api build docker-build

web-install:
	cd web && bun install --frozen-lockfile

web-build:
	cd web && bun run build

tauri-build: web-install
	cd web && \
	GH_TOKEN=$(GH_TOKEN) bunx tauri build

build-api:
	cd services && \
	GOWORK=off GONOSUMCHECK=github.com/alexander-bruun/orb/* \
	CGO_ENABLED=0 GOOS=linux go build -trimpath -o bin/api ./cmd/main.go

build: web-build build-api

docker-build:
	docker compose -f docker-compose.yml build

test:
	go test ./...

lint:
	golangci-lint run ./...
