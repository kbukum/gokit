.PHONY: all build test test-coverage lint vet fmt tidy update update-go check clean help \
       tag tag-push tag-force list-tags ci ci-test ci-lint ensure-act

GOMOD := ./gomod.sh

# Module flag: pass -m $(M) to gomod.sh when M is set
_M = $(if $(M),-m $(M))

## Default target
all: check

## Build packages (M=<module> for specific)
build:
	@$(GOMOD) cmd "go build" $(_M)

## Run tests (M=<module>, T=<test pattern>)
test:
	@$(GOMOD) cmd "go test -race -count=1 $(if $(T),-run $(T))" $(_M)

## Run tests with coverage (M=<module>, T=<test pattern>)
test-coverage:
	@$(GOMOD) cmd "go test -race -coverprofile=coverage.out -covermode=atomic $(if $(T),-run $(T))" $(_M)

## Run linter (M=<module>)
lint:
	@$(GOMOD) cmd "golangci-lint run" $(_M)

## Run go vet (M=<module>)
vet:
	@$(GOMOD) cmd "go vet" $(_M)

## Format code (M=<module>)
fmt:
ifdef M
	@echo "==> Formatting $(M)..."
	@gofmt -s -w $(M)
else
	@echo "==> Formatting..."
	@gofmt -s -w .
endif
	@echo "✓ Formatted"

## Tidy modules (M=<module>)
tidy:
	@$(GOMOD) tidy $(_M)

## Update dependencies (M=<module>)
update:
	@$(GOMOD) update $(_M)

## Update Go version across all modules (usage: make update-go VERSION=1.26.0)
update-go:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make update-go VERSION=1.26.0" && exit 1)
	@$(GOMOD) update-go $(VERSION)

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

## Run all checks (build + vet + test) — supports M=<module>
check: build vet test

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
	@act --secret GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(ACT_ARGS)

## Run only the test job from CI
ci-test: ensure-act
	@act -j test --secret GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(ACT_ARGS)

## Run only the lint job from CI
ci-lint: ensure-act
	@act -j lint --secret GITHUB_TOKEN=$$(gh auth token 2>/dev/null) $(ACT_ARGS)

## Show help
help:
	@echo "Usage: make <target> [M=<module>] [T=<test>]"
	@echo ""
	@echo "Development:"
	@echo "  make build    [M=]            Build packages"
	@echo "  make test     [M=] [T=]       Run tests"
	@echo "  make test-coverage [M=] [T=]  Run tests with coverage"
	@echo "  make lint     [M=]            Run golangci-lint"
	@echo "  make vet      [M=]            Run go vet"
	@echo "  make fmt      [M=]            Format code"
	@echo "  make tidy     [M=]            Run go mod tidy"
	@echo "  make update   [M=]            Update dependencies"
	@echo "  make check    [M=]            Build + vet + test"
	@echo "  make clean                    Remove build artifacts"
	@echo ""
	@echo "Go version:"
	@echo "  make update-go VERSION=1.26.0  Update Go version in all go.mod"
	@echo ""
	@echo "Versioning & Release:"
	@echo "  make tag VERSION=v0.1.0        Tag all modules"
	@echo "  make tag-push VERSION=v0.1.0   Tag and push to remote"
	@echo "  make tag-force VERSION=v0.1.0  Force overwrite tags"
	@echo "  make list-tags                 List all version tags"
	@echo ""
	@echo "Local CI (GitHub Actions via act + Docker):"
	@echo "  make ci                        Run full CI pipeline"
	@echo "  make ci-test                   Run only test job"
	@echo "  make ci-lint                   Run only lint job"
	@echo ""
	@echo "Module targeting (M=):"
	@echo "  M=kafka             Target kafka module"
	@echo "  M=httpclient/rest   Target httpclient module, rest package"
	@echo "  M=grpc/client       Target grpc module, client package"
	@echo "  M=security          Target root module, security package"
	@echo ""
	@echo "Examples:"
	@echo "  make test                            Test everything"
	@echo "  make test M=kafka                    Test kafka module"
	@echo "  make test M=httpclient/rest          Test rest subpackage"
	@echo "  make test M=httpclient T=TestClient  Test matching tests in httpclient"
	@echo "  make lint M=grpc                     Lint grpc module"
	@echo "  make check M=httpclient              Build+vet+test httpclient"