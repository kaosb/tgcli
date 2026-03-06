VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X github.com/kaosb/tgcli/cmd.version=$(VERSION)"

.PHONY: build run test lint clean install

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o tgcli .

install:
	CGO_ENABLED=1 go install $(LDFLAGS) .

run: build
	./tgcli $(ARGS)

test:
	CGO_ENABLED=1 go test ./... -cover

lint:
	go vet ./...
	golangci-lint run

clean:
	rm -f tgcli
