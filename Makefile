.PHONY: all run build test test-coverage bench fmt lint audit snapshot release-calc release-prepare release-dry changelog changelog-latest clean

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

# Calculate the next version based on unreleased changes
release-calc:
	@echo $(BUMPED_VERSION)

release-prepare:
	$(eval VERSION ?= $(BUMPED_VERSION))
	$(if $(strip $(VERSION)),,$(error VERSION cannot be empty))
	$(if $(findstring '",$(VERSION)),$(error VERSION cannot contain quotes))

	$(eval GIT_STATUS := $(shell git diff --quiet || echo dirty))
	$(if $(GIT_STATUS),$(error working directory needs to be clean))

	@echo "Preparing $(VERSION)..."

	@$(GIT_CLIFF_BIN) --output CHANGELOG.md --tag "$(VERSION)"

	@git diff --quiet CHANGELOG.md || \
		{ git add CHANGELOG.md && \
			git commit -m "chore(release): prepare for $(VERSION)" CHANGELOG.md; }

	git tag -a "$(VERSION)" -m "Release $(VERSION)"

	@echo "Done! Tagged $(VERSION)"
	@echo "Now push the commit (git push) and the tag (git push --tags)."

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
