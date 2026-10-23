.PHONY: test

test:
	go test -mod=readonly -coverprofile=coverage.out ./...
