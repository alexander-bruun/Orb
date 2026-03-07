DATABASE_URL       ?= postgres://orb:orb@localhost:5432/orb?sslmode=disable
KV_SENTINEL_ADDRS  ?= localhost:26379
KV_SENTINEL_MASTER ?= mymaster
STORE_BACKEND      ?= local
STORE_ROOT         ?= ./data/audio
HTTP_PORT          ?= 8080
DIR                ?= ./music
USER_ID            ?=

# Docker targets
docker-build:
	sudo docker compose build

docker-up: docker-build
	sudo docker compose up --remove-orphans

docker-down:
	sudo docker compose down -v

# Local targets
web-install:
	cd web && bun install

tauri-build: web-install
	cd web && \
	GH_TOKEN=$(GH_TOKEN) bunx tauri build

# Tauri Android targets (local)
tauri-android-init:
	. scripts/android.env && cd web && bunx tauri android init
	@$(MAKE) tauri-android-dev-sign

tauri-android-build:
	. scripts/android.env && cd web && bunx tauri android build --apk --aab

tauri-android-dev-sign:
	scripts/android-sign.sh --dev

# Tauri iOS targets (local)
tauri-ios-init:
	. scripts/ios.env && cd web && bunx tauri ios init
	@$(MAKE) tauri-ios-dev-sign

tauri-ios-build:
	. scripts/ios.env && cd web && bunx tauri ios build

tauri-ios-dev-sign:
	scripts/ios-sign.sh --dev

# Tauri CI targets (env already set by workflow)
tauri-android-init-ci:
	cd web && bunx tauri android init --ci
	@$(MAKE) tauri-patch-cleartext

tauri-android-build-ci:
	cd web && bunx tauri android build --apk --aab

tauri-ios-init-ci:
	cd web && bunx tauri ios init --ci

# Tauri's iOS build fails in CI due to code signing issues, I am looking into what to do.
tauri-ios-build-ci:
	cd web && TAURI_APPLE_DEVELOPMENT_TEAM=0000000000 APPLE_DEVELOPMENT_TEAM=0000000000 DEVELOPMENT_TEAM=0000000000 bunx tauri ios build || true

tauri-patch-cleartext:
	sed -i 's/manifestPlaceholders\["usesCleartextTraffic"\] = "false"/manifestPlaceholders["usesCleartextTraffic"] = "true"/' web/src-tauri/gen/android/app/build.gradle.kts

# Linting
lint:
	golangci-lint run ./...
