# Changelog

All notable changes to Lumen will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-05-28

Phase 3 release: operator customization, UI polish, i18n foundation, and clarified lightweight log direction.

### Added

- Runtime agent collection interval policy from hub settings, with agent polling/apply path and env/YAML bootstrap defaults.
- Parquet downsample policy controls in settings for bucket size and hot/cold/archive windows ahead of the cold-tier implementation.
- Product-grade UI polish across app shell, dashboard, host detail, settings, reusable surfaces, empty states, and onboarding-oriented host actions.
- Bilingual web UI foundation with English and Vietnamese runtime strings plus persisted language toggle.
- System metadata in host detail headers for hostname/IP, OS, uptime, kernel/arch, CPU model, and agent version context.

### Changed

- Docker agent onboarding is hub-first: create a host in the UI, then use the generated per-agent Docker Compose file instead of editing hub compose or per-agent config manually.
- Lightweight logs are explicitly deferred to a future dedicated Logs/Console surface with on-demand live streaming; logs must not be shipped through periodic metrics ingest or Host Detail polling.

## [0.1.0] - 2026-05-27

Initial public MVP release.

### Added

- Phase 0 project bootstrap: README, MIT license, contribution guide, GitHub templates, CI, release workflow, CodeQL workflow, Makefile, docs scaffold, and ADR-0001.
- Phase 1 technical spike: Go hub and agent, ingest endpoint, WebSocket live stream, embedded web build, Docker Compose path, source-run docs, OpenAPI spec, and REST Client examples.
- Phase 2 MVP breadth: authentication, host/token management, SQLite migrations, HDD-friendly batched persistence, metrics history API, retention settings, offline agent buffer, Docker collector, YAML agent config, host detail charts, PWA shell, install docs, reference docs, and FAQ.
- OSS readiness docs: Code of Conduct, Governance, Security Policy, Support guide, ADR-0002, and ADR-0003.

### Changed

- CodeQL workflow is gated behind manual dispatch while the staging repository remains private.

### Fixed

- golangci-lint CI configuration updated for golangci-lint v2 and the current GitHub Action version.
