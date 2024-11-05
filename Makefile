.EXPORT_ALL_VARIABLES:
.PHONY: test release run self-update

TAG = v$(shell grep 'const version' cmd/ru/main.go | sed -E 's/.*"(.+)"$$/\1/')

release:
	git tag $$TAG && git push origin $$TAG

test:
	go test -v ./...

run:
	go run cmd/ru/main.go update

self-update:
	go run cmd/ru/main.go self-update
