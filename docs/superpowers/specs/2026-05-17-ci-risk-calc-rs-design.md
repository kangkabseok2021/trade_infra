# CI: risk-calc-rs Rust Tests Design

**Date:** 2026-05-17
**Version:** v0.9.0

## Goal

Add a `rust-tests` job to `.github/workflows/risk-svc.yml` so the 9 Rust unit tests in `risk-svc/risk-calc-rs/` run in CI on every push/PR to `trade_infra/risk-svc/**`.

## Context

`risk-svc.yml` has two jobs: `cpp-tests` (CMake + ctest for C++ GoogleTests) and `go-tests` (Go CGo tests with libriskcalc). The 9 pure-Rust unit tests in `risk-calc-rs/src/lib.rs` are not in CI.

`risk-calc-rs` has no dependencies — pure math, no ureq, no serde_json. No `apt-get` installs required.

Pattern established by `market-data-svc.yml` `rust-tests` job (v0.8.0).

## Change

**File:** `.github/workflows/risk-svc.yml`

Add `rust-tests` job parallel to `cpp-tests` (no `needs:`):

```yaml
  rust-tests:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: trade_infra
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Run risk-calc-rs unit tests
        run: cargo test --manifest-path risk-svc/risk-calc-rs/Cargo.toml
```

## Properties

- Parallel to `cpp-tests` and `go-tests` (no dependency)
- Triggered by existing `on: push/pull_request paths: ["trade_infra/risk-svc/**"]`
- No DB, no Docker, no network, no extra apt installs
- Tests: `mtm_pnl_long_profit`, `mtm_pnl_long_loss`, `mtm_pnl_short`, `net_exposure_positive`, `net_exposure_negative`, `net_exposure_ignores_lmp`, `limit_breach_over`, `limit_breach_under`, `limit_breach_exact`

## Scope

- One file modified: `.github/workflows/risk-svc.yml`
- No code changes
