DATABASE_URL       ?= postgres://orb:orb@localhost:5432/orb?sslmode=disable
KV_SENTINEL_ADDRS  ?= localhost:26379
KV_SENTINEL_MASTER ?= mymaster
STORE_BACKEND      ?= local
STORE_ROOT         ?= ./data/audio
HTTP_PORT          ?= 8080
DIR                ?= ./music
USER_ID            ?=

# Capacitor targets
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

# Docker targets
docker-build:
	docker compose build

docker-up: docker-build
	sudo docker compose up --remove-orphans

docker-down:
	sudo docker compose down -v

# Local targets
web-install:
	cd web && npm install

tauri-build: web-install
	cd web && \
	GH_TOKEN=$(GH_TOKEN) npx tauri build

# Linting
lint:
	golangci-lint run ./...
