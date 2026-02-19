.PHONY: all build test lint vet fmt tidy check clean help update-go ci ci-test ci-lint ensure-act

GOMOD := ./gomod.sh

## Default target
all: check

## Build all packages
build:
	@echo "==> Building all modules..."
	@$(GOMOD) cmd "go build ./..."
	@echo "✓ All modules built"

## Run all tests
test:
	@echo "==> Testing all modules..."
	@$(GOMOD) cmd "go test -race -count=1 ./..."
	@echo "✓ All tests passed"

## Run tests with coverage
test-coverage:
	@echo "==> Testing all modules with coverage..."
	@$(GOMOD) cmd "go test -race -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -func=coverage.out | tail -1"

## Run linter
lint:
	@echo "==> Linting all modules..."
	@$(GOMOD) cmd "golangci-lint run ./..."
	@echo "✓ Lint passed"

## Run go vet
vet:
	@echo "==> Vetting all modules..."
	@$(GOMOD) cmd "go vet ./..."
	@echo "✓ Vet passed"

## Format code
fmt:
	@echo "==> Formatting..."
	@gofmt -s -w .
	@echo "✓ Formatted"

## Tidy all modules
tidy:
	@echo "==> Tidying all modules..."
	@$(GOMOD) tidy
	@echo "✓ All modules tidied"

## Update all dependencies
update:
	@echo "==> Updating all modules..."
	@$(GOMOD) update
	@echo "✓ All modules updated"

## Update Go version across all modules (usage: make update-go VERSION=1.26.0)
update-go:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make update-go VERSION=1.26.0" && exit 1)
	@echo "==> Updating Go version to $(VERSION) across all modules..."
	@$(GOMOD) update-go $(VERSION)
	@echo "✓ Go version updated to $(VERSION)"

## Tag all modules with a version (usage: make tag VERSION=v0.1.0)
tag:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make tag VERSION=v0.1.0" && exit 1)
	@./tag-modules.sh $(VERSION)

## Tag all modules and push to remote (usage: make tag-push VERSION=v0.1.0)
tag-push:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make tag-push VERSION=v0.1.0" && exit 1)
	@./tag-modules.sh $(VERSION) --push

## Tag all modules, overwriting existing (usage: make tag-force VERSION=v0.1.0)
tag-force:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make tag-force VERSION=v0.1.0" && exit 1)
	@./tag-modules.sh $(VERSION) --force

## List all version tags
list-tags:
	@echo "==> All version tags:"
	@git tag -l | sort -V

## Run all checks (build + vet + test)
check: build vet test
	@echo "✓ All checks passed"

## Clean build artifacts
clean:
	@find . -name "coverage.out" -o -name "coverage.html" | xargs rm -f
	@echo "✓ Cleaned"

## Ensure act is installed (auto-install via go install if missing)
ensure-act:
	@command -v act >/dev/null 2>&1 || { \
		echo "==> act not found, installing via go install..."; \
		go install github.com/nektos/act@latest; \
		echo "✓ act installed"; \
	}
	@command -v docker >/dev/null 2>&1 || { echo "Error: Docker is required but not installed. Please install Docker first." && exit 1; }

## Run full CI pipeline locally (mirrors GitHub Actions)
ci: ensure-act
	@echo "==> Running CI pipeline locally..."
	@act --secret GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(ACT_ARGS)
	@echo "✓ CI pipeline passed"

## Run only the test job from CI
ci-test: ensure-act
	@echo "==> Running CI test job locally..."
	@act -j test --secret GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(ACT_ARGS)
	@echo "✓ CI test job passed"

## Run only the lint job from CI
ci-lint: ensure-act
	@echo "==> Running CI lint job locally..."
	@act -j lint --secret GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(ACT_ARGS)
	@echo "✓ CI lint job passed"

## Show help
help:
	@echo "Available targets:"
	@echo "  make build                        - Build all modules"
	@echo "  make test                         - Run all tests"
	@echo "  make test-coverage                - Run tests with coverage report"
	@echo "  make lint                         - Run golangci-lint"
	@echo "  make vet                          - Run go vet"
	@echo "  make fmt                          - Format code with gofmt"
	@echo "  make tidy                         - Run go mod tidy on all modules"
	@echo "  make update                       - Run go get -u on all modules"
	@echo "  make update-go VERSION=1.26.0     - Update Go version in all go.mod files"
	@echo ""
	@echo "Versioning & Release:"
	@echo "  make tag VERSION=v0.1.0           - Tag all modules with version"
	@echo "  make tag-push VERSION=v0.1.0      - Tag all modules and push to remote"
	@echo "  make tag-force VERSION=v0.1.0     - Force overwrite existing tags"
	@echo "  make list-tags                    - List all version tags"
	@echo ""
	@echo "Quality:"
	@echo "  make check                        - Build + vet + test (CI check)"
	@echo "  make clean                        - Remove build artifacts"
	@echo ""
	@echo "Local CI (runs GitHub Actions locally via act + Docker):"
	@echo "  make ci                           - Run full CI pipeline locally"
	@echo "  make ci-test                      - Run only the test job"
	@echo "  make ci-lint                      - Run only the lint job"
	@echo "  make ci ACT_ARGS='--list'         - List available CI jobs"