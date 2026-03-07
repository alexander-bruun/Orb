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
	cd web && npm run dev

cap-build:
	cd web && npm run build

cap-sync: cap-build
	cd web && npx cap sync

cap-ios: cap-sync
	cd web && npx cap open ios

cap-android: cap-sync
	cd web && npx cap open android

cap-ios-build: cap-sync
	cd web && npx cap build ios

cap-android-build: cap-sync
	cd web && npx cap build android

# Run docker-compose build & up
docker-up:
	sudo docker-compose -f docker-compose.yml up --build --remove-orphans

docker-down:
	sudo docker-compose -f docker-compose.yml down -v

# Frontend / Tauri targets used by CI
.PHONY: web-install tauri-build docker-build

web-install:
	cd web && npm install

tauri-build: web-install
	cd web && \
	GH_TOKEN=$(GH_TOKEN) npx tauri build

docker-build:
	docker compose -f docker-compose.yml build

lint:
	golangci-lint run ./...
