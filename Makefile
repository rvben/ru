.EXPORT_ALL_VARIABLES:
.PHONY: test release run self-update build new-release

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

release:
	git tag v$(shell echo ${VERSION} | sed 's/^v//') && git push origin v$(shell echo ${VERSION} | sed 's/^v//')

test:
	go test -v ./...

run:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go update

self-update:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go self update
