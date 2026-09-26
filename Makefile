.PHONY: build test snapshot

build:
	go build -o E5SubBot .

test:
	go test ./...

snapshot:
	goreleaser release --snapshot --clean
