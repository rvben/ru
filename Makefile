.EXPORT_ALL_VARIABLES:
.PHONY: test release run self-update build new-release release-minor release-patch

# Get version from git tag, fallback to last tag + commit hash for dev builds
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.version=${VERSION}" -o bin/ru ./cmd/ru

# Create a new release with the specified version
new-release:
	@if [ -z "$(v)" ]; then \
		echo "Please specify version: make new-release v=0.1.54"; \
		exit 1; \
	fi
	@echo "Creating new release v$(v)"
	@if git rev-parse "v$(v)" >/dev/null 2>&1; then \
		echo "Version v$(v) already exists!"; \
		exit 1; \
	fi
	@echo "Creating and pushing tag v$(v)..."
	git tag "v$(v)"
	git push origin "v$(v)"
	@echo "Release workflow started. Check: https://github.com/rvben/ru/actions"

# Create a new minor release (increment the middle number)
release-minor:
	@echo "Creating new minor release..."
	@LATEST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'); \
	if [ -z "$$LATEST_TAG" ]; then \
		echo "No existing tags found. Please create an initial release with make new-release v=0.1.0"; \
		exit 1; \
	fi; \
	MAJOR=$$(echo $$LATEST_TAG | cut -d. -f1); \
	MINOR=$$(echo $$LATEST_TAG | cut -d. -f2); \
	PATCH=$$(echo $$LATEST_TAG | cut -d. -f3); \
	NEW_MINOR=$$((MINOR + 1)); \
	NEW_VERSION="$$MAJOR.$$NEW_MINOR.0"; \
	echo "Latest version: $$LATEST_TAG, New version: $$NEW_VERSION"; \
	$(MAKE) new-release v=$$NEW_VERSION

# Create a new patch release (increment the last number)
release-patch:
	@echo "Creating new patch release..."
	@LATEST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'); \
	if [ -z "$$LATEST_TAG" ]; then \
		echo "No existing tags found. Please create an initial release with make new-release v=0.1.0"; \
		exit 1; \
	fi; \
	MAJOR=$$(echo $$LATEST_TAG | cut -d. -f1); \
	MINOR=$$(echo $$LATEST_TAG | cut -d. -f2); \
	PATCH=$$(echo $$LATEST_TAG | cut -d. -f3); \
	NEW_PATCH=$$((PATCH + 1)); \
	NEW_VERSION="$$MAJOR.$$MINOR.$$NEW_PATCH"; \
	echo "Latest version: $$LATEST_TAG, New version: $$NEW_VERSION"; \
	$(MAKE) new-release v=$$NEW_VERSION

release:
	git tag v$(shell echo ${VERSION} | sed 's/^v//') && git push origin v$(shell echo ${VERSION} | sed 's/^v//')

test:
	go test -v ./...

run:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go update

self-update:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go self update
