# Docker targets
docker-build:
	sudo docker compose build

docker-up: docker-build
	sudo docker compose up --remove-orphans

docker-down:
	sudo docker compose down -v

# Tauri Windows targets (local)

# Tauri Linux targets (local)

# Tauri Mac targets (local)

# Tauri Android targets (local)
android-init:
	. scripts/android.env && cd web && bunx tauri android init
	@$(MAKE) android-dev-sign

android-build:
	. scripts/android.env && cd web && bunx tauri android build --apk --aab

android-dev-sign:
	scripts/android-sign.sh --dev

android-install:
	scripts/android-install.sh

# Tauri iOS targets (local)
ios-init:
	. scripts/ios.env && cd web && bunx tauri ios init
	@$(MAKE) ios-dev-sign

ios-build:
	. scripts/ios.env && cd web && bunx tauri ios build

ios-dev-sign:
	scripts/ios-sign.sh --dev

ios-install:
	scripts/ios-install.sh

# Helper functions for generating iOS and Android icons
icon-generate:
	cd web/src-tauri/ && cargo tauri icon icons/icon.png

# Linting
lint:
	golangci-lint run ./...
