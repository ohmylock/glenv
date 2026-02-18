.PHONY: build test lint install release release-check clean

VERSION ?= v0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/glenv ./cmd/glenv

test:
	go test -race ./...

lint:
	go vet ./...

install: build
	cp bin/glenv $(shell go env GOPATH)/bin/glenv

release:
	goreleaser release --clean

release-check:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf bin/ dist/ coverage.out
