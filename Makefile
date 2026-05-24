.DEFAULT_GOAL := help

.PHONY: help verify verify-fast verify-all fmt fmt-check lint test test-integration build smoke install run-fake run-fake-headless mod-check vuln doctor

GO_PACKAGES := ./...
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2} /^[a-zA-Z0-9_-]+:/ && !/##/ {next}' $(MAKEFILE_LIST) | grep -v '^$$'

verify: verify-fast smoke ## Full validation (fmt, lint, test, build, smoke)

verify-fast: fmt-check lint test build ## Quick validation without E2E smoke

verify-all: verify mod-check vuln ## Everything including mod tidy + vuln scan

fmt: ## Auto-format with gofumpt
	go tool gofumpt -w .

fmt-check: ## Fail if code is not gofumpt-formatted
	@test -z "$$(go tool gofumpt -l .)" || (echo "files need formatting — run: make fmt"; go tool gofumpt -l .; exit 1)

lint: ## go vet + golangci-lint
	go vet $(GO_PACKAGES)
	go run $(GOLANGCI_LINT) run $(GO_PACKAGES)

test: ## Unit tests with race detector
	go test $(GO_PACKAGES) -count=1 -race

test-integration: ## Full headless E2E (also covered by smoke)
	REVIEWSTACK_INTEGRATION=1 go test ./internal/app -count=1 -run TestHeadlessFakeRun

build: ## Compile binary to bin/reviewstack
	go build -o bin/reviewstack ./cmd/reviewstack

smoke: build ## Headless fake reviewer run
	./bin/reviewstack --uncommitted --fake-reviewers --no-tui

mod-check: ## Ensure go.mod and go.sum are tidy
	@go mod tidy
	@if git diff --quiet go.mod go.sum 2>/dev/null; then \
		echo "go.mod is tidy"; \
	else \
		echo "go.mod/go.sum changed after tidy — commit the diff"; \
		git diff go.mod go.sum; \
		exit 1; \
	fi

vuln: ## Scan for known vulnerabilities
	go tool govulncheck $(GO_PACKAGES)

install: ## Install to GOPATH/bin
	go install ./cmd/reviewstack

run-fake: build ## Launch TUI with fake reviewers
	./bin/reviewstack --pr 4914 --fake-reviewers

run-fake-headless: smoke ## Alias for smoke

doctor: build ## Check CLIs, auth, and config before a real run
	./bin/reviewstack --doctor
