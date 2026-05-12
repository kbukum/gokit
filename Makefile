.PHONY: all build test test-integration test-coverage lint vet fmt tidy update update-go check check-fast test-affected \
       check-core check-patterns check-crosscutting check-composition check-transport check-auth check-data check-ai \
       check-media check-infra clean help tag tag-push tag-force list-tags ci ci-test ci-lint ensure-act

GOMOD := ./gomod.sh

# Module flag: pass -m $(M) to gomod.sh when M is set
_M = $(if $(M),-m $(M))

# Workspace flag: pass -w $(W) to gomod.sh when W is set
_W = $(if $(W),-w $(W))

## Default target
all: check

## Build packages (M=<module> for specific, W=core|contrib for filtered workspace)
build:
	@$(GOMOD) cmd "go build" $(_M) $(_W)

## Run tests (M=<module>, T=<test pattern>, W=core|contrib)
test:
	@$(GOMOD) cmd "go test -race -shuffle=on -count=1 $(if $(T),-run $(T))" $(_M) $(_W)

## Run integration suite (gated by `//go:build integration`).
## Slow / dependency-heavy; not part of `make test` or default CI `check`.
test-integration:
	@$(GOMOD) cmd "go test -race -count=1 -tags=integration $(if $(T),-run $(T))" $(_M) $(_W)

## Run tests with coverage (M=<module>, T=<test pattern>, W=core|contrib)
test-coverage:
	@$(GOMOD) cmd "go test -race -coverprofile=coverage.out -covermode=atomic $(if $(T),-run $(T))" $(_M) $(_W)

## Run linter (M=<module>, W=core|contrib)
lint:
	@$(GOMOD) cmd "golangci-lint run" $(_M) $(_W)

## Run go vet (M=<module>, W=core|contrib)
vet:
	@$(GOMOD) cmd "go vet" $(_M) $(_W)

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

## Tidy modules (M=<module>, W=core|contrib)
tidy:
	@$(GOMOD) tidy $(_M) $(_W)

## Update dependencies (M=<module>, W=core|contrib)
update:
	@$(GOMOD) update $(_M) $(_W)

## Update Go version across modules (usage: make update-go VERSION=1.26.0 [W=core|contrib])
update-go:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make update-go VERSION=1.26.0" && exit 1)
	@$(GOMOD) update-go $(VERSION) $(_W)

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

## Fast check: build + vet + lint only (no tests) — for rapid iteration
check-fast: build vet lint

## Run tests only for modules affected by current changes (vs main branch)
test-affected:
	@echo "==> Detecting affected modules..."
	@CHANGED=$$(git diff --name-only origin/main...HEAD 2>/dev/null || git diff --name-only HEAD~1); \
	if [ -z "$$CHANGED" ]; then \
		echo "No changes detected, running all tests"; \
		$(GOMOD) cmd "go test -race -shuffle=on -count=1" $(_M) $(_W); \
	elif printf '%s\n' "$$CHANGED" | grep -Eq '^(go\.mod|go\.sum|go\.work|.*\.go\.work)$$'; then \
		echo "go.mod/go.sum or .go.work file changed, running all tests"; \
		$(GOMOD) cmd "go test -race -shuffle=on -count=1" $(_M) $(_W); \
	else \
		CHANGED=$$(printf '%s\n' "$$CHANGED" | grep -E '\.go$$|(^|/)(go\.mod|go\.sum)$$' || true); \
		if [ -z "$$CHANGED" ]; then \
			echo "No Go source changes"; \
			exit 0; \
		fi; \
		MODULES=$$(printf '%s\n' "$$CHANGED" | xargs -I{} dirname {} | sort -u | while read dir; do \
			if [ -f "$$dir/go.mod" ]; then echo "$$dir"; \
			else \
				d="$$dir"; \
				while [ "$$d" != "." ] && [ ! -f "$$d/go.mod" ]; do d=$$(dirname "$$d"); done; \
				[ -f "$$d/go.mod" ] && echo "$$d"; \
			fi; \
		done | sort -u); \
		if [ -z "$$MODULES" ]; then \
			echo "No Go module changes detected"; \
		else \
			if [ -n "$(W)" ]; then \
				if [ ! -f "$(W).go.work" ]; then \
					echo "Error: workspace file not found: $(W).go.work"; \
					exit 1; \
				fi; \
				WS_MODS=$$(awk '/^use[[:space:]]*\(/{u=1;next} u&&/\)/{u=0;next} u{gsub(/^[[:space:]]+|[[:space:]]+$$/,"");if($$0!="")print} /^use[[:space:]]+[^(]/{sub(/^use[[:space:]]+/,"");gsub(/^[[:space:]]+|[[:space:]]+$$/,"");print}' $(W).go.work); \
				FILTERED=""; \
				for mod in $$MODULES; do \
					for wmod in $$WS_MODS; do \
						wmod=$${wmod#./}; wmod=$${wmod%/}; \
						if [ "$$mod" = "$$wmod" ] || [ "$$mod" = "." -a "$$wmod" = "" ]; then \
							FILTERED="$$FILTERED $$mod"; \
							break; \
						fi; \
					done; \
				done; \
				MODULES=$$FILTERED; \
				if [ -z "$$MODULES" ]; then \
					echo "No affected modules in workspace $(W)"; \
					exit 0; \
				fi; \
			fi; \
			echo "Affected modules: $$MODULES"; \
			failed=0; \
			for mod in $$MODULES; do \
				echo "==> Testing $$mod..."; \
				if ! $(GOMOD) cmd "go test -race -shuffle=on -count=1" -m "$$mod" $(_W); then \
					echo "✗ Tests failed in $$mod"; \
					failed=1; \
				fi; \
			done; \
			if [ "$$failed" -ne 0 ]; then \
				exit 1; \
			fi; \
		fi; \
	fi

## Run all checks (build + vet + test) — supports M=<module>
check: build vet test

## Check only core domain modules
check-core:
	@./scripts/check-domain.sh core

## Check only patterns domain modules
check-patterns:
	@./scripts/check-domain.sh patterns

## Check only crosscutting domain modules
check-crosscutting:
	@./scripts/check-domain.sh crosscutting

## Check only composition domain modules
check-composition:
	@./scripts/check-domain.sh composition

## Check only transport domain modules
check-transport:
	@./scripts/check-domain.sh transport

## Check only auth domain modules
check-auth:
	@./scripts/check-domain.sh auth

## Check only data domain modules
check-data:
	@./scripts/check-domain.sh data

## Check only ai domain modules
check-ai:
	@./scripts/check-domain.sh ai

## Check only media domain modules
check-media:
	@./scripts/check-domain.sh media

## Check only infra domain modules
check-infra:
	@./scripts/check-domain.sh infra

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
	@echo "Usage: make <target> [M=<module>] [T=<test>] [W=core|contrib]"
	@echo ""
	@echo "Development:"
	@echo "  make help                     Show this help"
	@echo "  make build    [M=] [W=]       Build packages"
	@echo "  make test     [M=] [T=] [W=]  Run tests"
	@echo "  make test-affected [M=] [W=]  Run tests for changed modules vs main"
	@echo "  make test-integration [M=] [W=] Run integration suite (//go:build integration)"
	@echo "  make test-coverage [M=] [T=] [W=] Run tests with coverage"
	@echo "  make lint     [M=] [W=]       Run golangci-lint"
	@echo "  make vet      [M=] [W=]       Run go vet"
	@echo "  make fmt      [M=]            Format code"
	@echo "  make tidy     [M=] [W=]       Run go mod tidy"
	@echo "  make update   [M=] [W=]       Update dependencies"
	@echo "  make check-fast [M=] [W=]     Build + vet + lint"
	@echo "  make check    [M=] [W=]       Build + vet + test"
	@echo "  make check-core               Check only core domain modules"
	@echo "  make check-patterns           Check only patterns domain modules"
	@echo "  make check-crosscutting       Check only crosscutting domain modules"
	@echo "  make check-composition        Check only composition domain modules"
	@echo "  make check-transport          Check only transport domain modules"
	@echo "  make check-auth               Check only auth domain modules"
	@echo "  make check-data               Check only data domain modules"
	@echo "  make check-ai                 Check only ai domain modules"
	@echo "  make check-media              Check only media domain modules"
	@echo "  make check-infra              Check only infra domain modules"
	@echo "  make clean                    Remove build artifacts"
	@echo ""
	@echo "Go version:"
	@echo "  make update-go VERSION=1.26.0 [W=]  Update Go version in go.mod files"
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
	@echo "  M=messaging         Target messaging module"
	@echo "  M=httpclient/rest   Target httpclient module, rest package"
	@echo "  M=grpc/client       Target grpc module, client package"
	@echo "  M=security          Target root module, security package"
	@echo ""
	@echo "Workspace targeting (W=):"
	@echo "  W=core              Only core modules"
	@echo "  W=contrib           Only contrib/adapter modules"
	@echo ""
	@echo "Examples:"
	@echo "  make test                            Test everything"
	@echo "  make test M=messaging                Test messaging module"
	@echo "  make test M=httpclient/rest          Test rest subpackage"
	@echo "  make test M=httpclient T=TestClient  Test matching tests in httpclient"
	@echo "  make test W=core                     Test only core modules"
	@echo "  make build W=contrib                 Build only contrib modules"
	@echo "  make lint M=grpc                     Lint grpc module"
	@echo "  make check M=httpclient              Build+vet+test httpclient"
