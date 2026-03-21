# Frontend
web-install:
	cd web && bun install

# Tauri desktop build (installs frontend deps first, native target)
tauri-build: web-install
	cd web && bunx tauri build

# Docker targets
docker-build:
	sudo docker compose build

docker-up: docker-build
	sudo docker compose up --remove-orphans

docker-down:
	sudo docker compose down -v

# Tauri Windows targets (local)
windows-build: web-install
	cd web && bunx tauri build --target x86_64-pc-windows-msvc

windows-build-arm: web-install
	cd web && bunx tauri build --target aarch64-pc-windows-msvc

# Tauri Linux targets (local)
linux-build: web-install
	cd web && bunx tauri build --target x86_64-unknown-linux-gnu

linux-build-arm: web-install
	cd web && bunx tauri build --target aarch64-unknown-linux-gnu

# Tauri Mac targets (local)
macos-build: web-install
	cd web && bunx tauri build --target x86_64-apple-darwin

macos-build-arm: web-install
	cd web && bunx tauri build --target aarch64-apple-darwin

macos-build-universal: web-install
	cd web && bunx tauri build --target universal-apple-darwin

# Tauri Android targets (local)
android-init:
	. scripts/android.env && cd web && cargo tauri android init
	@$(MAKE) android-dev-sign

android-build:
	. scripts/android.env && cd web && cargo tauri android build --apk --aab

android-dev-sign:
	scripts/android-sign.sh --dev

android-install:
	scripts/android-install.sh

# Tauri iOS targets (local)
ios-init:
	. scripts/ios.env && cd web && cargo tauri ios init
	@$(MAKE) ios-dev-sign

ios-build:
	. scripts/ios.env && cd web && cargo tauri ios build

ios-dev-sign:
	scripts/ios-sign.sh --dev

ios-install:
	scripts/ios-install.sh

# CI targets (no local env sourcing, no dev signing — env vars set by CI)
android-init-ci:
	cd web && bunx tauri android init

android-build-ci:
	cd web && bunx tauri android build --apk --aab

ios-init-ci:
	cd web && bunx tauri ios init

ios-build-ci:
	cd web && bunx tauri ios build

# Helper functions for generating iOS and Android icons
icon-generate:
	cd web/src-tauri/ && cargo tauri icon icons/icon.png

# Backend build targets
.PHONY: backend-build backend-build-tagged test lint
backend-build:
	cd services && go build -o ../orb github.com/alexander-bruun/orb/services/cmd

backend-build-tagged:
	$(eval GIT_TAG := $(shell git describe --tags --always 2>/dev/null || echo "unknown"))
	$(eval GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown"))
	cd services && go build -ldflags "-X main.buildTag=$(GIT_TAG) -X main.buildSHA=$(GIT_SHA)" -o ../orb github.com/alexander-bruun/orb/services/cmd

# Testing
test:
	cd services && go test -v ./...

# Linting
lint:
	golangci-lint run ./...
