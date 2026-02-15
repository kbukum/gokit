.PHONY: all build test lint vet fmt tidy check clean help

# Sub-modules with their own go.mod
SUBMODULES := database redis kafka storage server grpc discovery connect llm transcription diarization

## Default target
all: check

## Build all packages
build:
	@echo "==> Building core..."
	@go build ./...
	@for mod in $(SUBMODULES); do \
		echo "==> Building $$mod..."; \
		cd $$mod && go build ./... && cd ..; \
	done
	@echo "✓ All modules built"

## Run all tests
test:
	@echo "==> Testing core..."
	@go test -race -count=1 ./...
	@for mod in $(SUBMODULES); do \
		echo "==> Testing $$mod..."; \
		cd $$mod && go test -race -count=1 ./... && cd ..; \
	done
	@echo "✓ All tests passed"

## Run tests with coverage
test-coverage:
	@echo "==> Testing core with coverage..."
	@go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -1
	@echo ""
	@for mod in $(SUBMODULES); do \
		echo "==> Testing $$mod with coverage..."; \
		cd $$mod && go test -race -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -func=coverage.out | tail -1 && cd ..; \
		echo ""; \
	done

## Run linter
lint:
	@echo "==> Linting core..."
	@golangci-lint run ./...
	@for mod in $(SUBMODULES); do \
		echo "==> Linting $$mod..."; \
		cd $$mod && golangci-lint run ./... && cd ..; \
	done
	@echo "✓ Lint passed"

## Run go vet
vet:
	@echo "==> Vetting core..."
	@go vet ./...
	@for mod in $(SUBMODULES); do \
		echo "==> Vetting $$mod..."; \
		cd $$mod && go vet ./... && cd ..; \
	done
	@echo "✓ Vet passed"

## Format code
fmt:
	@echo "==> Formatting..."
	@gofmt -s -w .
	@echo "✓ Formatted"

## Tidy all modules
tidy:
	@echo "==> Tidying core..."
	@go mod tidy
	@for mod in $(SUBMODULES); do \
		echo "==> Tidying $$mod..."; \
		cd $$mod && go mod tidy && cd ..; \
	done
	@echo "✓ All modules tidied"

## Run all checks (build + vet + test)
check: build vet test
	@echo "✓ All checks passed"

## Clean build artifacts
clean:
	@rm -f coverage.out coverage.html
	@for mod in $(SUBMODULES); do \
		rm -f $$mod/coverage.out $$mod/coverage.html; \
	done
	@echo "✓ Cleaned"

## Show help
help:
	@echo "Available targets:"
	@echo "  make build          - Build all packages"
	@echo "  make test           - Run all tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make lint           - Run golangci-lint"
	@echo "  make vet            - Run go vet"
	@echo "  make fmt            - Format code with gofmt"
	@echo "  make tidy           - Run go mod tidy on all modules"
	@echo "  make check          - Build + vet + test (CI check)"
	@echo "  make clean          - Remove build artifacts"
