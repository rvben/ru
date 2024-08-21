.EXPORT_ALL_VARIABLES:
.PHONY: test

# Get tag from main.go
TAG = $(shell grep -o '[0-9]\.[0-9]\.[0-9]' main.go)

release:
	git tag $$TAG && git push origin $$TAG -f

test:
	go test -v ./...