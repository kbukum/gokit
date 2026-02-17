# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2024-05-22

### Changed
- **errors**: Modernized module with comprehensive godoc comments and 100% test coverage.
- **errors**: Internal errors are no longer retryable by default for improved safety.
- **core**: Consolidated core packages (errors, util, validation, etc.) into the root module.
- **component**: Renamed `ComponentHealth` to `Health` to avoid stuttering.
- **config**: Renamed `ConfigResolver` to `Resolver` to avoid stuttering.
- **server**: Renamed `ServerComponent` to `Component` to avoid stuttering.
- **resilience**: Updated `ExecuteWithResult` to accept `context.Context` as the first parameter.
- **various**: Updated multiple functions to accept configuration by pointer to improve performance and satisfy linters.

## [Unreleased]

### Added
- Core module: errors, config, logger, util, version, encryption, validation, di, resilience, observability, sse, provider, component, bootstrap
- Sub-modules: database, redis, kafka, storage, server, grpc, discovery, connect, llm, transcription, diarization
- Provider pattern with generic Registry/Manager/Selector for LLM, transcription, diarization
- Unified HTTP server (Gin + h2c) with middleware and endpoint sub-packages
- Connect-Go integration module for RPC alongside REST
- Multi-module architecture: core stays lightweight, heavy deps in sub-modules
