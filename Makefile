.PHONY: all run build test test-coverage bench fmt lint audit clean

# GOEXPERIMENT=jsonv2 required for encoding/json/jsontext
# Enables streaming JSON transformation with preserved formatting.
GO := GOEXPERIMENT=jsonv2 go

BINARY_NAME := claudine
MAIN := ./cmd/claudine

all: test build

run:
	$(GO) run $(MAIN)

build:
	$(GO) build -o $(BINARY_NAME) $(MAIN)

test:
	$(GO) test -race ./...

# Use -coverprofile for unit tests; -test.gocoverdir is for integration tests with built binaries
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

bench:
	$(GO) test -bench=. -benchmem ./...

fmt:
	$(GO) fmt ./...

lint:
	GOEXPERIMENT=jsonv2 golangci-lint run

audit:
	$(GO) vet ./...
	GO tool govulncheck ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
