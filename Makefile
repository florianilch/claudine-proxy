.PHONY: all run build test test-coverage bench fmt lint audit snapshot release-calc changelog changelog-latest clean

# GOEXPERIMENT=jsonv2 required for encoding/json/jsontext
# Enables streaming JSON transformation with preserved formatting.
GO := GOEXPERIMENT=jsonv2 go

GIT_CLIFF_BIN := bunx git-cliff@2.10.1 -c cliff.config.toml

BINARY_NAME := claudine
MAIN := ./cmd/claudine

# Calculated next version
BUMPED_VERSION = $(shell $(GIT_CLIFF_BIN) --bumped-version)

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
	go tool govulncheck ./...

# Build for all platforms using GoReleaser (local testing)
snapshot:
	goreleaser build --snapshot --clean

# Simulate full release with GoReleaser (includes archives, checksums)
release-dry:
	goreleaser release --snapshot --clean

# Write full changelog
changelog:
	$(GIT_CLIFF_BIN) --output CHANGELOG.md

# Generate changelog for latest tag (current or most recent)
changelog-latest:
	$(GIT_CLIFF_BIN) --strip all --latest

clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -rf ./dist/
