SHELL := /bin/sh

.PHONY: build check clean fmt fmt-check test vet

build:
	mkdir -p bin
	go build -trimpath -o bin/nccli ./cmd/nccli

check: fmt-check
	go mod verify
	go run ./internal/tools/textcheck .
	go vet ./...
	go test -race ./...
	go build ./...

clean:
	rm -rf ./bin ./dist

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || \
		{ echo "Go files need formatting. Run make fmt." >&2; exit 1; }

test:
	go test -race ./...

vet:
	go vet ./...
