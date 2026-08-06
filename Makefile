# vertc — VolcEngine AI audio/video developer-workflow CLI
BIN      := vertc
PKG      := github.com/volcengine/VolcEngineRTC_CLI
VERSION  ?= 0.0.1-dev
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Single identity source of truth lives in internal/meta. Rename the
# binary by overriding BIN here or with an ldflags -X on meta.BinName.
LDFLAGS := -X '$(PKG)/internal/meta.Version=$(VERSION)' \
	       -X '$(PKG)/internal/meta.ReleaseMarker=VERTC_RELEASE_VERSION=$(VERSION);' \
           -X '$(PKG)/internal/meta.Commit=$(COMMIT)' \
           -X '$(PKG)/internal/meta.BuildDate=$(DATE)' \
           -X '$(PKG)/internal/meta.BinName=$(BIN)'

# Pinned development and release toolchain.
GOLANGCI_VERSION := v1.62.2
GOLANGCI         = $(shell go env GOPATH)/bin/golangci-lint
GORELEASER_VERSION := v2.17.0
GORELEASER         = $(shell go env GOPATH)/bin/goreleaser

# Repository-specific CI extensions may add prerequisites without changing the
# portable public build definition.
-include .make/ci-extra.mk

.PHONY: build test test-node vet fmt fmt-check lint check-error-codes skills-check check-change-contract check-change-contract-test check-release-files release-tools release-snapshot release-snapshot-test ci ci-go e2e tools install clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BIN) .

test:
	go test ./...

test-node:
	node --test scripts/*.test.js

vet:
	go vet ./...

fmt:
	gofmt -w .

# Fail if any file is not gofmt-clean (CI gate).
fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

lint:
	@test -x "$(GOLANGCI)" || { echo "golangci-lint not found — run 'make tools'"; exit 1; }
	$(GOLANGCI) run ./...

check-error-codes:
	./scripts/snapshot-error-codes.sh

# Skill quality gate: command authenticity against the live command tree, plus
# metadata/links/secrets/banned-legacy-command checks. Also runs inside `make
# test` via `go test ./...`; this is the focused alias.
skills-check:
	go test ./internal/skillscan/... -count=1

check-change-contract:
	./scripts/check-change-contract.sh

check-change-contract-test:
	./scripts/check-change-contract_test.sh

check-release-files:
	@for file in LICENSE NOTICE README.md go.mod; do \
		test -f "$$file" || { echo "missing release file: $$file"; exit 1; }; \
	done

release-tools:
	go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

release-snapshot:
	@test -x "$(GORELEASER)" || { echo "goreleaser not found — run 'make release-tools'"; exit 1; }
	GORELEASER="$(GORELEASER)" ./scripts/build-release-artifacts.sh --stability snapshot --publication none --output dist

release-snapshot-test:
	GORELEASER="$(GORELEASER)" ./scripts/release-snapshot_integration_test.sh

# The Go-only gate used by CI jobs whose image intentionally has no Node.js.
ci-go: fmt-check vet lint test check-error-codes check-change-contract-test check-change-contract build

# The complete configured local gate.
ci: ci-go $(CI_EXTRA_TARGETS) test-node

e2e:
	go test ./tests/... -count=1

# Install pinned dev tools.
tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_VERSION)

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -rf bin
