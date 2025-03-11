.EXPORT_ALL_VARIABLES:
.PHONY: build test clean fmt check doc version-get version-major version-minor version-patch version-push release-major release-minor release-patch run self-update

# Get version from git tag, fallback to last tag + commit hash for dev builds
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.version=${VERSION}" -o bin/ru ./cmd/ru

test:
	go test -v ./...

clean:
	go clean

fmt:
	go fmt ./...

check:
	go vet ./...
	go list -json ./... | grep -v /vendor/ | xargs -n 1 go vet

doc:
	go doc -all ./...

run:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go update

self-update:
	go run -ldflags "-X main.version=${VERSION}" cmd/ru/main.go self update

# Version tagging targets
version-get:
	@echo "Current version: $$(git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)"

version-major:
	@echo "Creating new major version tag..."
	$(eval CURRENT := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0))
	$(eval MAJOR := $(shell echo $(CURRENT) | sed -E 's/v([0-9]+)\.[0-9]+\.[0-9]+/\1/'))
	$(eval NEW_MAJOR := $(shell echo $$(( $(MAJOR) + 1 ))))
	$(eval NEW_TAG := v$(NEW_MAJOR).0.0)
	@echo "Current: $(CURRENT) -> New: $(NEW_TAG)"
	@git commit --allow-empty -m "Bump version to $(NEW_TAG)"
	@git tag -a $(NEW_TAG) -m "Release $(NEW_TAG)"
	@echo "Version $(NEW_TAG) created and committed. Run 'make version-push' to trigger release workflow."

version-minor:
	@echo "Creating new minor version tag..."
	$(eval CURRENT := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0))
	$(eval MAJOR := $(shell echo $(CURRENT) | sed -E 's/v([0-9]+)\.[0-9]+\.[0-9]+/\1/'))
	$(eval MINOR := $(shell echo $(CURRENT) | sed -E 's/v[0-9]+\.([0-9]+)\.[0-9]+/\1/'))
	$(eval NEW_MINOR := $(shell echo $$(( $(MINOR) + 1 ))))
	$(eval NEW_TAG := v$(MAJOR).$(NEW_MINOR).0)
	@echo "Current: $(CURRENT) -> New: $(NEW_TAG)"
	@git commit --allow-empty -m "Bump version to $(NEW_TAG)"
	@git tag -a $(NEW_TAG) -m "Release $(NEW_TAG)"
	@echo "Version $(NEW_TAG) created and committed. Run 'make version-push' to trigger release workflow."

version-patch:
	@echo "Creating new patch version tag..."
	$(eval CURRENT := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0))
	$(eval MAJOR := $(shell echo $(CURRENT) | sed -E 's/v([0-9]+)\.[0-9]+\.[0-9]+/\1/'))
	$(eval MINOR := $(shell echo $(CURRENT) | sed -E 's/v[0-9]+\.([0-9]+)\.[0-9]+/\1/'))
	$(eval PATCH := $(shell echo $(CURRENT) | sed -E 's/v[0-9]+\.[0-9]+\.([0-9]+)/\1/'))
	$(eval NEW_PATCH := $(shell echo $$(( $(PATCH) + 1 ))))
	$(eval NEW_TAG := v$(MAJOR).$(MINOR).$(NEW_PATCH))
	@echo "Current: $(CURRENT) -> New: $(NEW_TAG)"
	@git commit --allow-empty -m "Bump version to $(NEW_TAG)"
	@git tag -a $(NEW_TAG) -m "Release $(NEW_TAG)"
	@echo "Version $(NEW_TAG) created and committed. Run 'make version-push' to trigger release workflow."

# Target to push the new tag and changes automatically
version-push:
	$(eval LATEST_TAG := $(shell git describe --tags --abbrev=0))
	@echo "Pushing latest commit and tag $(LATEST_TAG) to origin..."
	@git push
	@git push origin $(LATEST_TAG)
	@echo "Release workflow triggered for $(LATEST_TAG)"

# Combined targets for one-step release
release-major: version-major version-push
release-minor: version-minor version-push
release-patch: version-patch version-push

# For compatibility with previous Makefile
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
