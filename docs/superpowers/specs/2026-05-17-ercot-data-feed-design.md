# ERCOT Data Feed Design

**Date:** 2026-05-17
**Version:** v0.7.0

## Goal

Replace the Ornstein-Uhlenbeck LMP generator in `tick-engine` (Rust cdylib) with historical ERCOT DAM Settlement Point Price data, keeping the `extern "C"` ABI identical so `market-data-svc` C++ consumer requires zero changes.

## Context

`trade_infra` v0.6.0 has a Rust cdylib (`market-data-svc/tick-engine/`) that implements:

```c
TickGenerator* tick_generator_create(double base_lmp, double volatility, unsigned int seed);
void           tick_generator_next(TickGenerator* gen, double* out_lmp, double* out_load_mw);
void           tick_generator_destroy(TickGenerator* gen);
```

`market-data-svc/src/main.cpp` calls `tick_generator_create(base_lmp, volatility, 42)` once at startup, then loops calling `tick_generator_next` + sleeping `INTERVAL_MS`. It reads `NODE_NAME` from env (e.g. `HB_NORTH`).

The ABI must remain unchanged. All new behaviour is inside the Rust crate.

## Architecture

### Modified file

| File | Change |
|------|--------|
| `market-data-svc/tick-engine/Cargo.toml` | Add `ureq = "2"`, `serde_json = "1"` |
| `market-data-svc/tick-engine/src/lib.rs` | Add ERCOT fetch + replay logic |
| `infra/docker-compose.yml` | Add `ERCOT_REPLAY_DATE: "2024-01-15"` to 3 market-data-svc services |

No changes to: `market-data-svc/src/main.cpp`, `include/marketdata.h`, `CMakeLists.txt`, `Dockerfile`, or any other service.

## Data Source

**ERCOT OpenData API — DAM Settlement Point Prices:**

```
GET https://api.ercot.com/api/public-reports/np4-190-cd/dam_stlmnt_pnt_prices
    ?deliveryDateFrom=2024-01-15
    &deliveryDateTo=2024-01-15
    &settlementPoint=HB_NORTH
    &size=96
```

Returns JSON with a `data` array of arrays. Each inner array is positional:
- `[0]` deliveryDate
- `[1]` deliveryHour (1–24)
- `[2]` deliveryInterval (1–4, 15-min sub-intervals)
- `[3]` settlementPointName
- `[4]` settlementPointPrice (LMP, $/MWh)

96 rows per node per day (24 hours × 4 intervals). Parser extracts field `[4]` in row order → `Vec<f64>`.

Settlement point mapping (direct, 1:1):
- `HB_NORTH` → `"HB_NORTH"`
- `HB_SOUTH` → `"HB_SOUTH"`
- `HB_WEST`  → `"HB_WEST"`

Unknown `NODE_NAME` values → empty buffer → OU fallback.

## `TickGenerator` Struct Changes

```rust
pub struct TickGenerator {
    // ERCOT replay fields
    lmps: Vec<f64>,   // empty = OU fallback active
    idx:  usize,

    // OU fields — kept for load_mw and LMP fallback
    base_lmp:   f64,
    volatility: f64,
    lmp:        f64,
    rng:        rand::rngs::SmallRng,
    dist:       Normal<f64>,
}
```

## Data Flow

```
tick_generator_create(base_lmp, volatility, seed)
  │
  ├── read NODE_NAME env var (default "HB_NORTH")
  ├── read ERCOT_REPLAY_DATE env var (default "2024-01-15")
  ├── call fetch_ercot_lmps(settlement_point, date)
  │     └── ureq GET → parse_ercot_json(body) → Option<Vec<f64>>
  ├── if Some(lmps): store in struct, log "loaded N LMPs for <node>"
  └── if None: lmps = vec![], log "ERCOT fetch failed, using OU fallback"

tick_generator_next(gen, out_lmp, out_load_mw)
  │
  ├── if lmps non-empty:
  │     *out_lmp = lmps[idx % lmps.len()]
  │     idx += 1
  └── else (OU fallback):
        *out_lmp = OU formula (existing)
  │
  └── *out_load_mw = OU load formula (always, ERCOT data has no load)
```

## New Functions

```rust
/// Pure parser — no I/O. Extracts field [4] from each row in data array.
fn parse_ercot_json(body: &str) -> Option<Vec<f64>>;

/// HTTP fetch + parse. Returns None on any error (timeout, non-200, bad JSON, 0 rows).
fn fetch_ercot_lmps(settlement_point: &str, date: &str) -> Option<Vec<f64>>;
```

Both are private (no `pub`). Only `tick_generator_create/next/destroy` are `#[no_mangle] pub extern "C"`.

## Error Handling

- Any error in `fetch_ercot_lmps` (network, non-200, parse failure, empty result) → returns `None`
- `tick_generator_create` treats `None` as OU fallback — no panic, process continues
- Startup message written to stderr:
  - Success: `tick-engine: loaded 96 ERCOT LMPs for HB_NORTH (2024-01-15)`
  - Fallback: `tick-engine: ERCOT fetch failed for HB_NORTH, using OU fallback`

## docker-compose.yml

Add `ERCOT_REPLAY_DATE: "2024-01-15"` to the environment of:
- `market-data-svc`
- `market-data-svc-south`
- `market-data-svc-west`

`NODE_NAME` is already set on each service and maps directly to the ERCOT settlement point.

## Testing

All tests in `market-data-svc/tick-engine/src/lib.rs` under `#[cfg(test)]`:

| Test | What it verifies |
|------|-----------------|
| `parse_lmps_extracts_prices` | `parse_ercot_json` with 3-row JSON → `vec![p1, p2, p3]` |
| `parse_lmps_returns_none_on_bad_json` | Malformed JSON → `None` |
| `replay_cycles_buffer` | `lmps=[10,20,30]`, call `next` 4× → 4th LMP == 10.0 (wrap) |
| `replay_uses_ou_when_empty` | `lmps=[]`, call `next` 100× → LMP stays in `[1.0, 499.0]` |
| `lmp_stays_in_range` | (existing) OU path range check — still passes |
| `deterministic_with_same_seed` | (existing) OU determinism — still passes |

`fetch_ercot_lmps` is not unit tested (HTTP). It is verified by Docker integration in Task 6 (check logs for "loaded 96 ERCOT LMPs").

## Scope Boundaries

- No changes to C++ consumer, CMakeLists.txt, or Dockerfile
- `load_mw` always uses OU (ERCOT DAM data has no per-node load)
- Single replay date shared across all three nodes (same calendar day, different hub prices)
- No live polling, no background threads
- `base_lmp`, `volatility`, `seed` args to `tick_generator_create` are kept for OU fallback; `base_lmp` is also used as the OU mean-reversion target when in fallback mode
