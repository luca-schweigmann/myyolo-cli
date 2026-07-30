.PHONY: build check format test

build:
	go build -trimpath -o bin/myyolo ./cmd/myyolo

format:
	gofmt -w cmd internal

test:
	go test -race -count=1 ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	go mod verify
	go vet ./...
	go test -race -count=1 ./...
	go build -trimpath -o bin/myyolo ./cmd/myyolo
