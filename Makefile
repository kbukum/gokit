.PHONY: all build test test-integration test-coverage lint vet fmt tidy update update-go check check-fast test-affected structure \
       check-core check-patterns check-crosscutting check-composition check-transport check-auth check-data check-ai \
       check-media check-infra check-devtools clean help release-plan release-status release-readiness release-tag \
       release-publish-dry-run release-publish list-tags release-dry ci ci-test ci-lint ensure-act toven-canary module-index release-bump

GOMOD := ./gomod.sh
# Candidate Toven binary for the read-only self-hosting canary. Defaults to a
# `toven` on PATH; point it at a freshly built binary to dogfood a candidate,
# e.g. `make TOVEN=../toven/target/release/toven toven-canary`.
TOVEN ?= toven

# Module flag: pass -m $(M) to gomod.sh when M is set
_M = $(if $(M),-m $(M))

# Workspace flag: pass -w $(W) to gomod.sh when W is set
_W = $(if $(W),-w $(W))

# Non-empty when a module (M=) or workspace (W=) subset is requested. The
# everyday gates (build/test/lint/vet/fmt/tidy/coverage) run the WHOLE workspace
# through Toven — the argv-first orchestrator that discovers modules, orders by
# the dependency graph, and fans out across cores. A specific M=/W= subset falls
# back to the native gomod.sh path: Toven has no first-class selector for
# gokit's named domains yet (see docs/TOVEN-MIGRATION.md, gap 3).
_FILTERED = $(strip $(M)$(W))

## Default target
all: check

## Build packages. Unfiltered → Toven (whole workspace); M=/W= → native gomod.sh.
build:
ifeq ($(_FILTERED),)
	@$(TOVEN) build
else
	@$(GOMOD) cmd "go build" $(_M) $(_W)
endif

## Run tests. Unfiltered → Toven (whole workspace, single wave); M=/W= → native.
## T=<pattern> passes through as `-run` either way.
test:
ifeq ($(_FILTERED),)
	@$(TOVEN) test -- -race -shuffle=on -count=1 $(if $(T),-run $(T))
else
	@$(GOMOD) cmd "go test -race -shuffle=on -count=1 $(if $(T),-run $(T))" $(_M) $(_W)
endif

## Run integration suite (gated by `//go:build integration`).
## Slow / dependency-heavy; not part of `make test` or default CI `check`.
test-integration:
	@$(GOMOD) cmd "go test -race -count=1 -tags=integration $(if $(T),-run $(T))" $(_M) $(_W)

## Run tests with coverage. Unfiltered → Toven's coverage gate; M=/W= → native.
test-coverage:
ifeq ($(_FILTERED),)
	@$(TOVEN) coverage -- -race -covermode=atomic $(if $(T),-run $(T))
else
	@$(GOMOD) cmd "go test -race -coverprofile=coverage.out -covermode=atomic $(if $(T),-run $(T))" $(_M) $(_W)
endif

## Run linter. Unfiltered → Toven; M=/W= → native gomod.sh.
lint:
ifeq ($(_FILTERED),)
	@$(TOVEN) lint
else
	@$(GOMOD) cmd "golangci-lint run" $(_M) $(_W)
endif

## Run go vet (Toven's `check` task). Unfiltered → Toven; M=/W= → native.
vet:
ifeq ($(_FILTERED),)
	@$(TOVEN) check
else
	@$(GOMOD) cmd "go vet" $(_M) $(_W)
endif

## Format code. Unfiltered → Toven's `format` task; M=<module> → native gofmt.
fmt:
ifdef M
	@echo "==> Formatting $(M)..."
	@gofmt -s -w $(M)
	@echo "✓ Formatted"
else
	@$(TOVEN) format
endif

## Tidy modules. Unfiltered → Toven's `tidy-fix` (mutating); M=/W= → native.
tidy:
ifeq ($(_FILTERED),)
	@$(TOVEN) tidy-fix
else
	@$(GOMOD) tidy $(_M) $(_W)
endif

## Update dependencies (M=<module>, W=core|contrib)
update:
	@$(GOMOD) update $(_M) $(_W)

## Update Go version across modules (usage: make update-go VERSION=1.26.0 [W=core|contrib])
update-go:
	@[ -n "$(VERSION)" ] || (echo "Error: VERSION is required. Usage: make update-go VERSION=1.26.0" && exit 1)
	@$(GOMOD) update-go $(VERSION) $(_W)

## Preview the release plan: selected modules, versions, tags, and order (read-only)
release-plan:
	@$(TOVEN) release plan

## Report release status: policies, declared versions, tags, published versions (read-only)
release-status:
	@$(TOVEN) release status

## Fail-closed release go/no-go checks (read-only)
release-readiness:
	@$(TOVEN) release readiness

## Phase 1 — bump: rewrite every module's version + inter-module dependency floors
## (the lock-step `require github.com/kbukum/gokit/<mod> vX.Y.Z` lines) and, where
## configured, roll the CHANGELOG, then STAGE the mutation WITHOUT committing.
## Run it on a clean `main`. Toven never commits, tags, pushes, or publishes here:
## the maintainer rotates the `[Unreleased]` CHANGELOG heading, cuts a
## `release/vX.Y.Z` branch carrying the staged bump, and opens a PR. The signed
## tags are cut by `release-tag` only after that PR merges into `main`.
release-bump:
	@$(TOVEN) release bump --yes

## Phase 2 — tag (run only after the release-bump PR merges into `main`): create and
## push the signed lock-step module tags on the merged commit. Toven owns tagging
## and commit-derived release notes. (usage: make release-tag)
release-tag:
	@$(TOVEN) release tag --yes

## Mutation-free registry + hosted-Release rehearsal (read-only)
release-publish-dry-run:
	@$(TOVEN) release publish --dry-run

## Authoritative release: tag, push, and create the hosted GitHub Release; the pushed tag then
## triggers .github/workflows/release.yml (GoReleaser) to attach source archive + SBOM + signatures.
release-publish:
	@$(TOVEN) release publish --yes

## List all version tags
list-tags:
	@echo "==> All version tags:"
	@git tag -l | sort -V

## Read-only Toven self-hosting canary: discover modules and the dependency
## graph, then exercise the mutation-free release previews (status + plan) with
## the candidate Toven binary. Toven owns the authoritative release tag/publish
## path; GoReleaser only attaches supply-chain artifacts to the Toven-created
## Release. Toven never fabricates a synthetic 0.0.0, so before the first version
## tag the release previews fail closed with "no reachable release tag" — that is
## the expected outcome and is tolerated here. Once the first version tag exists
## the previews succeed instead; that success is the expected steady state and is
## reported as a pass. Only a failure for any other reason fails the canary.
## TOVEN selects the binary (see the TOVEN var).
toven-canary:
	@$(TOVEN) modules
	@$(TOVEN) graph
	@for verb in status plan; do \
	  echo ">> toven release $$verb (mutation-free preview)"; \
	  if out="$$($(TOVEN) release $$verb 2>&1)"; then \
	    printf '%s\n' "$$out"; \
	    echo "release $$verb now succeeds — a release tag is reachable"; \
	  elif printf '%s\n' "$$out" | grep -q 'no reachable release tag'; then \
	    echo "release $$verb fails closed as expected — no reachable release tag before the first Go release"; \
	  else \
	    printf '%s\n' "$$out"; \
	    echo "error: release $$verb failed for an unexpected reason" >&2; \
	    exit 1; \
	  fi; \
	done

## Release dry-run: build source archive + SBOM + checksums without signing or
## publishing (usage: make release-dry [VERSION=v0.1.0-alpha.1]). Requires
## goreleaser + cyclonedx-gomod on PATH.
release-dry:
	@command -v goreleaser >/dev/null 2>&1 || { echo "Error: goreleaser not found — install with: go install github.com/goreleaser/goreleaser/v2@latest"; exit 1; }
	@command -v cyclonedx-gomod >/dev/null 2>&1 || { echo "Error: cyclonedx-gomod not found — install with: go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest"; exit 1; }
	@GORELEASER_CURRENT_TAG=$(if $(VERSION),$(VERSION),v0.0.0-dev) goreleaser release --snapshot --clean --skip=sign,publish

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

## Verify declare-only aggregators (doc.go docs-only) + god-file — advisory, not gating.
## Driven through Toven's `command` ecosystem (`toven run structure`), which shells out
## to scripts/check-structure.sh — one source of truth, invoked by both make and CI.
structure:
	@$(TOVEN) run structure

## Regenerate docs/MODULE-INDEX.md from the discovered modules via Toven's
## `command` ecosystem (`toven run module-index` → scripts/generate-module-index.sh).
module-index:
	@$(TOVEN) run module-index

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

## Check only devtools domain modules
check-devtools:
	@./scripts/check-domain.sh devtools

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
	@echo "  make check-devtools           Check only devtools domain modules"
	@echo "  make clean                    Remove build artifacts"
	@echo ""
	@echo "Guardrails (Toven-driven):"
	@echo "  make structure                Declare-only/god-file guard (advisory)"
	@echo "  make module-index             Regenerate docs/MODULE-INDEX.md"
	@echo ""
	@echo "Go version:"
	@echo "  make update-go VERSION=1.26.0 [W=]  Update Go version in go.mod files"
	@echo ""
	@echo "Versioning & Release (Toven owns tag + publish):"
	@echo "  make release-plan              Preview the release plan (read-only)"
	@echo "  make release-status            Report release status (read-only)"
	@echo "  make release-readiness         Fail-closed release go/no-go checks"
	@echo "  make release-bump              Phase 1: stage version + CHANGELOG bump (no commit) for a PR"
	@echo "  make release-tag               Phase 2: create + push signed module tags (after bump PR merges)"
	@echo "  make release-publish-dry-run   Mutation-free registry + hosted-Release rehearsal"
	@echo "  make release-publish           Authoritative release: tag, push, and hosted Release"
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
