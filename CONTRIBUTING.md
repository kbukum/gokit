# Contributing to GoKit

## Development Setup

```bash
git clone https://github.com/skillsenselab/gokit.git
cd gokit
make build   # build all modules
make test    # run all tests
make lint    # run linter (requires golangci-lint)
```

## Project Structure

- **Core packages** (`errors/`, `config/`, `logger/`, etc.) share the root `go.mod`
- **Sub-modules** (`database/`, `redis/`, `server/`, etc.) have their own `go.mod`

## Making Changes

1. Create a feature branch from `main`
2. Make your changes
3. Run `make check` (builds, vets, tests everything)
4. Submit a pull request

## Adding a New Core Package

1. Create `yourpkg/` directory with `.go` files (package name = directory name)
2. Only import from other core packages or standard library + lightweight deps
3. Add a `README.md` with install, usage example, and key types
4. Run `go mod tidy` at the root

## Adding a New Sub-Module

1. Create `yourmod/` directory with its own `go.mod`:
   ```
   module github.com/skillsenselab/gokit/yourmod
   require github.com/skillsenselab/gokit v0.0.0
   replace github.com/skillsenselab/gokit => ../
   ```
2. Add `config.go` with `Config` struct, `ApplyDefaults()`, `Validate()`
3. Add `component.go` implementing `component.Component` if it has lifecycle
4. Add the module name to `SUBMODULES` in `Makefile`
5. Add a `README.md`

## Adding a Provider Implementation

For modules using the provider pattern (llm, transcription, diarization, storage, discovery):

1. Create a subfolder: `llm/openai/`
2. Package name = folder name
3. Import parent package for interfaces/types
4. Export a `Factory()` function for registry integration
5. Never import sibling providers

## Coding Standards

- `gofmt` and `goimports` formatting
- Each module owns its config (`Config` struct with `ApplyDefaults()` and `Validate()`)
- Plain Go validation for config (no external validator library)
- Provider implementations in subfolders, not flat files
- Features of a module in subfolders (e.g., `kafka/producer/`, `server/middleware/`)

## Versioning

Core module and each sub-module version independently using git tags:
- Core: `v0.1.0`, `v0.2.0`
- Sub-modules: `database/v0.1.0`, `server/v0.1.0`
