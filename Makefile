.EXPORT_ALL_VARIABLES:
.PHONY: test release run self-update build

# Get version from git tag, fallback to last tag + commit hash for dev builds
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.version=${VERSION}" -o bin/ru ./cmd/ru

release:
	git tag v$(shell echo ${VERSION} | sed 's/^v//') && git push origin v$(shell echo ${VERSION} | sed 's/^v//')

test:
	go test -v ./...

run:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go update

self-update:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go self-update
