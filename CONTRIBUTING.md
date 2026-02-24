# Contributing to gokit

## Prerequisites

- **Go 1.25+**
- **golangci-lint** — `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- **Docker** — required only for `make ci` (local CI via [act](https://github.com/nektos/act))

## Getting Started

```bash
git clone https://github.com/kbukum/gokit.git
cd gokit
make check   # build + vet + test across all modules
make lint    # run golangci-lint across all modules
```

## Project Structure

gokit is a **multi-module Go toolkit**. It provides reusable building blocks — not applications.

```
gokit/
├── go.mod                  # Root module (core packages)
├── errors/                 # ─┐
├── config/                 #  │
├── logger/                 #  │ Core packages — share root go.mod
├── provider/               #  │ Zero heavy dependencies
├── pipeline/               #  │
├── resilience/             # ─┘
├── database/               # ─┐
│   ├── go.mod              #  │ Sub-modules — own go.mod
│   └── testutil/           #  │ Bring in heavier deps (GORM, Kafka, etc.)
│       └── go.mod          #  │ Each has a `replace ../` for local dev
├── redis/                  #  │
├── kafka/                  #  │
├── server/                 # ─┘
├── gomod.sh                # Cross-module operations script
├── tag-modules.sh          # Multi-module tagging script
├── Makefile                # Developer workflow targets
└── .golangci.yml           # Shared linter configuration
```

**Core packages** live under the root `go.mod` and must stay lightweight.
**Sub-modules** each have their own `go.mod` and may pull in heavy dependencies.

## Development Workflow

### Make Targets

All targets support `M=<module>` for targeting a specific module:

```bash
make check                   # build + vet + test everything
make test                    # test all modules
make test M=redis            # test only the redis module
make test M=redis T=TestGet  # run specific test in redis
make lint                    # lint all modules
make lint M=provider         # lint only provider
make tidy                    # go mod tidy across all modules
make fmt                     # format all code
make ci                      # run full CI pipeline locally (requires Docker)
```

### Cross-Module Script

`gomod.sh` discovers all modules automatically by finding `go.mod` files. You never maintain a hardcoded module list:

```bash
./gomod.sh tidy              # go mod tidy all modules
./gomod.sh cmd "go test"     # run tests across all modules
./gomod.sh cmd "go test" -m kafka  # run tests in kafka only
```

## Making Changes

1. Create a feature branch from `main`
2. Make your changes
3. Run `make check` — builds, vets, and tests everything
4. Run `make lint` — ensure no linter issues
5. Run `make tidy` — keep go.mod/go.sum clean
6. Submit a pull request

## Adding a New Core Package

1. Create `yourpkg/` at the repo root with a `doc.go` for package documentation
2. Only import standard library, other core packages, or lightweight dependencies
3. Add tests (`yourpkg/*_test.go`) — aim for high coverage
4. Run `make tidy` at the root

## Adding a New Sub-Module

1. Create `yourmod/` with its own `go.mod`:
   ```
   module github.com/kbukum/gokit/yourmod
   go 1.25.0
   require github.com/kbukum/gokit v0.1.2
   replace github.com/kbukum/gokit => ../
   ```
2. Add a `doc.go` with package-level documentation
3. If the module wraps an infrastructure component, implement `component.Component` for lifecycle management
4. Add tests — the module is automatically discovered by `gomod.sh`, CI, and all `make` targets

## Coding Standards

- **Formatting**: `gofmt` and `goimports` (enforced by CI via `.golangci.yml`)
- **Imports**: Separate stdlib, third-party, and gokit imports with blank lines
- **Config pattern**: Each module that needs configuration uses a `Config` struct with `ApplyDefaults()` and `Validate()` methods
- **Validation**: Plain Go validation — no external validator library
- **Naming**: Follow Go conventions; avoid stuttering (e.g., `server.Component` not `server.ServerComponent`)
- **Testing**: Use `-race -count=1`; use table-driven tests where appropriate

## Versioning & Releases

All modules are tagged together at the same version:

```bash
make tag-push VERSION=v0.2.0   # tag all modules and push
make list-tags                  # view all tags
```

Tags are created per module (e.g., `v0.2.0`, `redis/v0.2.0`, `kafka/v0.2.0`) by `tag-modules.sh`, which auto-discovers modules. See [docs/VERSIONING.md](docs/VERSIONING.md) for the full guide.

## CI

CI runs on GitHub Actions and is fully dynamic — modules are discovered at runtime, not hardcoded. Each module gets its own parallel check and lint job. You can run it locally:

```bash
make ci        # full pipeline (requires Docker + act)
make ci-test   # test jobs only
make ci-lint   # lint jobs only
```
