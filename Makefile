.EXPORT_ALL_VARIABLES:
.PHONY: test

TAG = v$(shell grep 'const version' main.go | sed -E 's/.*"(.+)"$$/\1/')

release:
	git tag $$TAG && git push origin $$TAG -f

test:
	go test -v ./...