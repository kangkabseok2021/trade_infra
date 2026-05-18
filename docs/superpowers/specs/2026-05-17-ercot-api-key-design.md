# ERCOT API Key Support Design

**Date:** 2026-05-17
**Version:** v0.10.0

## Goal

Add `ERCOT_API_KEY` env var support to `fetch_ercot_lmps` so the ERCOT DAM SPP API request includes the `Ocp-Apim-Subscription-Key` header required by the Incapsula WAF.

## Context

`market-data-svc/tick-engine/src/lib.rs` fetches ERCOT LMP data at startup via `fetch_ercot_lmps`. Since v0.7.0 the fetch always returns 403 because the ERCOT public API requires `Ocp-Apim-Subscription-Key` header for programmatic access. The OU fallback activates and ticks still flow, but real data is never loaded.

API key obtained at: https://developer.ercot.com

## Change

### `market-data-svc/tick-engine/src/lib.rs`

Replace the `ureq` call in `fetch_ercot_lmps` (lines 65–70):

**Before:**
```rust
let body = ureq::get(&url)
    .timeout(std::time::Duration::from_secs(5))
    .call()
    .ok()?
    .into_string()
    .ok()?;
```

**After:**
```rust
let api_key = std::env::var("ERCOT_API_KEY").unwrap_or_default();
let body = ureq::get(&url)
    .set("Ocp-Apim-Subscription-Key", &api_key)
    .timeout(std::time::Duration::from_secs(5))
    .call()
    .ok()?
    .into_string()
    .ok()?;
```

`unwrap_or_default()` returns an empty string when `ERCOT_API_KEY` is unset. The ERCOT API returns 403 for an empty/invalid key → `.call().ok()?` → `None` → OU fallback. Behaviour when key is absent is identical to v0.7.0.

### `infra/docker-compose.yml`

Add `ERCOT_API_KEY: ""` to the environment block of all three `market-data-svc` services:

```yaml
      ERCOT_REPLAY_DATE: "2024-01-15"
      ERCOT_API_KEY: ""
```

Users replace `""` with their key to enable real ERCOT data.

## No test changes

The 6 existing Rust unit tests do not exercise the HTTP layer. `replay_uses_ou_when_empty` already covers the fallback path. The empty-key → 403 → fallback path is not unit-testable without a live API, and the fallback behaviour is already verified by the existing test.

## Scope

- Two files modified: `market-data-svc/tick-engine/src/lib.rs`, `infra/docker-compose.yml`
- No Cargo.toml changes (ureq `.set()` is part of the existing ureq 2 API)
- No CI changes
