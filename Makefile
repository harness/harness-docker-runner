.PHONY: test

test:
	go test ./... -covermode=atomic -coverprofile=coverage.out
