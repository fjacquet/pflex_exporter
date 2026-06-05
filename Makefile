BIN     = pflex_exporter
DIST    = dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)
# Cross-compilation targets are defined in .goreleaser.yaml (builds.goos/goarch).

# Pinned tool versions (installed by `make tools`).
GOLANGCI_LINT_VERSION   ?= v2.12.2
CYCLONEDX_GOMOD_VERSION ?= latest
GOVULNCHECK_VERSION     ?= latest

all: cli test docker

# Install pinned dev/CI tooling into $(GOBIN)/$GOPATH/bin.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Just the SBOM generator — used by the release pipeline (GoReleaser sboms hook).
tools-sbom:
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)

# --- quality gates (used by CI) ---

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed in:"; gofmt -l .; exit 1)

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test ./...

test-race:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

test-coverage: test-race
	go tool cover -html=coverage.out -o coverage.html

vuln:
	govulncheck ./...

# Aggregate gate run by CI.
ci: fmt-check vet lint test-race vuln

# Local convenience: format, vet, test, build, lint.
sure: fmt vet test
	go build ./...
	golangci-lint run

# --- artifacts ---

cli:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BIN) .

# CycloneDX SBOM for the Go module (source/dependency SBOM).
sbom:
	@mkdir -p $(DIST)
	cyclonedx-gomod mod -licenses -json -output $(DIST)/sbom.cdx.json
	@echo "wrote $(DIST)/sbom.cdx.json"

# Cross-compiled binaries + archives + SBOM + checksums + GitHub Release.
# Driven by GoReleaser (.goreleaser.yaml). Real releases run from a `v*` tag in CI;
# this target reproduces that pipeline locally — needs a tag and GITHUB_TOKEN.
release: tools-sbom
	goreleaser release --clean

# Local dry-run: full pipeline (build, archive, SBOM, checksums) without publishing.
release-snapshot: tools-sbom
	goreleaser release --snapshot --clean
	@echo "release artifacts in $(DIST)/"

docker:
	docker build -t $(BIN):$(VERSION) -t $(BIN):latest .

run-cli: cli
	./bin/$(BIN) --config config.yaml

clean-dist:
	rm -rf $(DIST)

clean: clean-dist
	rm -f bin/$(BIN) coverage.out coverage.html

.PHONY: all tools tools-sbom fmt-check fmt vet lint test test-race test-coverage vuln ci sure \
        cli sbom release release-snapshot docker run-cli clean-dist clean
