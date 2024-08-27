.EXPORT_ALL_VARIABLES:
.PHONY: test release run

TAG = v$(shell grep 'const version' cmd/ru/main.go | sed -E 's/.*"(.+)"$$/\1/')

release:
	git tag $$TAG && git push origin $$TAG -f

test:
	go test -v ./...

run:
	go run cmd/ru/main.go update