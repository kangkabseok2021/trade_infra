# CI: tick-engine Rust Tests Design

**Date:** 2026-05-17
**Version:** v0.8.0

## Goal

Add a `rust-tests` job to `.github/workflows/market-data-svc.yml` so the 6 Rust unit tests in `market-data-svc/tick-engine/` run in CI on every push/PR to `market-data-svc/**`.

## Context

`market-data-svc.yml` currently has one job (`build-and-test`) that runs CMake + ctest for the C++ GoogleTest suite. Since v0.7.0, `tick-engine/src/lib.rs` has 6 pure Rust unit tests (`cargo test`) that are not covered by CI.

`risk-svc.yml` establishes the pattern: multiple parallel jobs per workflow, `dtolnay/rust-toolchain@stable` for explicit Rust setup.

## Change

**File:** `.github/workflows/market-data-svc.yml`

Add a second job `rust-tests` that runs in parallel with the existing `build-and-test` job:

```yaml
  rust-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Run tick-engine unit tests
        run: cargo test --manifest-path market-data-svc/tick-engine/Cargo.toml
```

## Properties

- Parallel to `build-and-test` (no `needs:` dependency)
- Triggered by the same `on: push/pull_request paths: ["market-data-svc/**"]` already on the workflow
- No DB, no Docker, no network — pure unit tests
- `--manifest-path` avoids `cd` and works from the repo root

## Scope

- One file modified: `.github/workflows/market-data-svc.yml`
- No code changes
- `risk-svc/risk-calc-rs/` has the same gap — out of scope here, follow-up
