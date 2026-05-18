# ERCOT Data Feed Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Ornstein-Uhlenbeck LMP generator in the `tick-engine` Rust cdylib with historical ERCOT DAM Settlement Point Price data, keeping the `extern "C"` ABI identical.

**Architecture:** `tick_generator_create` reads `NODE_NAME` and `ERCOT_REPLAY_DATE` from env, calls `fetch_ercot_lmps` (ureq HTTP → JSON parse), and stores the result in a `Vec<f64>` field on `TickGenerator`. `tick_generator_next` cycles through that buffer for LMP; if the buffer is empty (fetch failed), it falls back to the existing OU formula. `load_mw` always uses OU. No C++ consumer changes.

**Tech Stack:** Rust 2021, `ureq = "2"` (sync HTTP with TLS), `serde_json = "1"`, existing `rand 0.8` + `rand_distr 0.4`, Docker Compose.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `market-data-svc/tick-engine/Cargo.toml` | Modify | Add `ureq`, `serde_json` deps |
| `market-data-svc/tick-engine/src/lib.rs` | Modify | Struct fields, parse fn, fetch fn, updated create/next |
| `infra/docker-compose.yml` | Modify | `ERCOT_REPLAY_DATE` on 3 market-data-svc services |

---

## Task 1: Add `ureq` and `serde_json` to Cargo.toml

**Files:**
- Modify: `market-data-svc/tick-engine/Cargo.toml`

- [ ] **Step 1: Open `market-data-svc/tick-engine/Cargo.toml` and add two dependencies**

The current `[dependencies]` section has only `rand` and `rand_distr`. Add `ureq` and `serde_json`:

```toml
[package]
name = "tick-engine"
version = "0.1.0"
edition = "2021"

[lib]
name = "marketdata"
crate-type = ["cdylib"]

[dependencies]
rand       = { version = "0.8", features = ["small_rng"] }
rand_distr = "0.4"
ureq       = "2"
serde_json = "1"
```

- [ ] **Step 2: Verify the crate compiles with the new deps**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/market-data-svc/tick-engine
cargo build 2>&1 | tail -5
```

Expected: `Compiling tick-engine ...` followed by `Finished`. If `ureq` needs TLS headers on macOS, it will link against the system's Security framework automatically.

- [ ] **Step 3: Commit**

```bash
git add market-data-svc/tick-engine/Cargo.toml
git commit -m "build(tick-engine): add ureq and serde_json dependencies"
git push origin main
```

---

## Task 2: Write 4 failing tests + add struct stubs

**Files:**
- Modify: `market-data-svc/tick-engine/src/lib.rs`

This task adds the two new struct fields, updates `new()`, adds a stub `parse_ercot_json` that always returns `None`, and adds 4 tests. The parse tests fail because the stub returns `None`. The replay tests fail because `next()` still uses OU (not the buffer). The 2 existing tests still pass (they use `new()` which has empty `lmps`, exercising OU).

- [ ] **Step 1: Replace `market-data-svc/tick-engine/src/lib.rs` with this content**

```rust
use rand::SeedableRng;
use rand_distr::{Distribution, Normal};

const THETA: f64     = 0.1;
const BASE_LOAD: f64 = 15_000.0;
const LOAD_VOL: f64  = 500.0;
const LMP_MIN: f64   = 1.0;
const LMP_MAX: f64   = 499.0;
const LOAD_MIN: f64  = 5_001.0;

pub struct TickGenerator {
    lmps:      Vec<f64>,
    idx:       usize,
    base_lmp:  f64,
    volatility: f64,
    lmp:       f64,
    rng:       rand::rngs::SmallRng,
    dist:      Normal<f64>,
}

impl TickGenerator {
    pub fn new(base_lmp: f64, volatility: f64, seed: u32) -> Self {
        Self {
            lmps:      vec![],
            idx:       0,
            base_lmp,
            volatility,
            lmp:       base_lmp,
            rng:       rand::rngs::SmallRng::seed_from_u64(seed as u64),
            dist:      Normal::new(0.0, 1.0).unwrap(),
        }
    }

    pub fn next(&mut self, out_lmp: &mut f64, out_load_mw: &mut f64) {
        self.lmp += THETA * (self.base_lmp - self.lmp)
            + self.volatility * self.dist.sample(&mut self.rng);
        self.lmp = self.lmp.clamp(LMP_MIN, LMP_MAX);
        *out_lmp = self.lmp;
        let load = BASE_LOAD + LOAD_VOL * self.dist.sample(&mut self.rng);
        *out_load_mw = load.max(LOAD_MIN);
    }
}

fn parse_ercot_json(_body: &str) -> Option<Vec<f64>> {
    None  // stub — implemented in Task 3
}

// --- extern "C" ABI — same symbols as the former C++ libmarketdata.so ---

#[no_mangle]
pub extern "C" fn tick_generator_create(
    base_lmp: f64,
    volatility: f64,
    seed: u32,
) -> *mut TickGenerator {
    Box::into_raw(Box::new(TickGenerator::new(base_lmp, volatility, seed)))
}

#[no_mangle]
pub extern "C" fn tick_generator_destroy(gen: *mut TickGenerator) {
    if !gen.is_null() {
        unsafe { drop(Box::from_raw(gen)); }
    }
}

#[no_mangle]
pub extern "C" fn tick_generator_next(
    gen: *mut TickGenerator,
    out_lmp: *mut f64,
    out_load_mw: *mut f64,
) {
    if gen.is_null() || out_lmp.is_null() || out_load_mw.is_null() {
        return;
    }
    unsafe { (*gen).next(&mut *out_lmp, &mut *out_load_mw); }
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- existing tests (must keep passing) ---

    #[test]
    fn lmp_stays_in_range() {
        let mut g = TickGenerator::new(45.0, 5.0, 42);
        for _ in 0..1000 {
            let (mut lmp, mut load) = (0.0f64, 0.0f64);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0, "lmp={lmp} out of [1,499]");
            assert!(load > 5000.0, "load={load} not > 5000");
        }
    }

    #[test]
    fn deterministic_with_same_seed() {
        let mut g1 = TickGenerator::new(45.0, 5.0, 42);
        let mut g2 = TickGenerator::new(45.0, 5.0, 42);
        for i in 0..20 {
            let (mut l1, mut d1, mut l2, mut d2) = (0.0f64, 0.0f64, 0.0f64, 0.0f64);
            g1.next(&mut l1, &mut d1);
            g2.next(&mut l2, &mut d2);
            assert_eq!(l1, l2, "lmp diverged at tick {i}");
        }
    }

    // --- new tests (failing until Tasks 3 and 4) ---

    #[test]
    fn parse_lmps_extracts_prices() {
        let json = r#"{"data":[["2024-01-15",1,1,"HB_NORTH",23.45],["2024-01-15",1,2,"HB_NORTH",24.10],["2024-01-15",1,3,"HB_NORTH",22.80]]}"#;
        let result = parse_ercot_json(json);
        assert_eq!(result, Some(vec![23.45, 24.10, 22.80]));
    }

    #[test]
    fn parse_lmps_returns_none_on_bad_json() {
        assert_eq!(parse_ercot_json("not json"), None);
    }

    #[test]
    fn replay_cycles_buffer() {
        // Construct directly with a known lmps buffer
        let mut g = TickGenerator {
            lmps:      vec![10.0, 20.0, 30.0],
            idx:       0,
            base_lmp:  45.0,
            volatility: 5.0,
            lmp:       45.0,
            rng:       rand::rngs::SmallRng::seed_from_u64(42),
            dist:      Normal::new(0.0, 1.0).unwrap(),
        };
        let (mut lmp, mut load) = (0.0f64, 0.0f64);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 10.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 20.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 30.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 10.0, "should wrap around");
    }

    #[test]
    fn replay_uses_ou_when_empty() {
        // Empty lmps → falls back to OU → LMP stays in [1, 499]
        let mut g = TickGenerator {
            lmps:      vec![],
            idx:       0,
            base_lmp:  45.0,
            volatility: 5.0,
            lmp:       45.0,
            rng:       rand::rngs::SmallRng::seed_from_u64(42),
            dist:      Normal::new(0.0, 1.0).unwrap(),
        };
        for _ in 0..100 {
            let (mut lmp, mut load) = (0.0f64, 0.0f64);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0, "lmp={lmp} out of [1,499]");
            assert!(load > 5000.0, "load={load} not > 5000");
        }
    }
}
```

- [ ] **Step 2: Run tests — 2 existing pass, 4 new fail**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/market-data-svc/tick-engine
cargo test 2>&1 | tail -20
```

Expected output:
```
test tests::lmp_stays_in_range ... ok
test tests::deterministic_with_same_seed ... ok
test tests::parse_lmps_extracts_prices ... FAILED
test tests::parse_lmps_returns_none_on_bad_json ... FAILED
test tests::replay_cycles_buffer ... FAILED
test tests::replay_uses_ou_when_empty ... FAILED

test result: FAILED. 2 passed; 4 failed
```

`parse_lmps_extracts_prices` fails because stub returns `None` (expected `Some([23.45, ...])`).
`parse_lmps_returns_none_on_bad_json` fails because stub returns `None` (passes? — actually this one passes because the stub returns `None` for all input, and the expected is also `None`).

Wait — `parse_lmps_returns_none_on_bad_json` expects `None`, and the stub returns `None` → this test actually PASSES at the stub stage. That's fine; it will still pass after real implementation.

So expected failing: `parse_lmps_extracts_prices`, `replay_cycles_buffer`. The `replay_uses_ou_when_empty` also passes at this stage (empty lmps + OU → in range). Corrected expected output:

```
test tests::lmp_stays_in_range ... ok
test tests::deterministic_with_same_seed ... ok
test tests::parse_lmps_returns_none_on_bad_json ... ok
test tests::replay_uses_ou_when_empty ... ok
test tests::parse_lmps_extracts_prices ... FAILED
test tests::replay_cycles_buffer ... FAILED

test result: FAILED. 4 passed; 2 failed
```

`parse_lmps_extracts_prices` fails: stub returns `None`, expected `Some([23.45, 24.10, 22.80])`.
`replay_cycles_buffer` fails: `next()` uses OU, not the buffer, so first call returns ~45.0, not 10.0.

- [ ] **Step 3: Commit**

```bash
git add market-data-svc/tick-engine/src/lib.rs
git commit -m "test(tick-engine): add 4 ERCOT replay tests with struct stubs"
git push origin main
```

---

## Task 3: Implement `parse_ercot_json`

**Files:**
- Modify: `market-data-svc/tick-engine/src/lib.rs`

Replace the stub `parse_ercot_json` with the real JSON parser. Everything else stays the same as Task 2.

- [ ] **Step 1: Replace the stub `parse_ercot_json` with the real implementation**

Find this line in `lib.rs`:
```rust
fn parse_ercot_json(_body: &str) -> Option<Vec<f64>> {
    None  // stub — implemented in Task 3
}
```

Replace it with:
```rust
fn parse_ercot_json(body: &str) -> Option<Vec<f64>> {
    let v: serde_json::Value = serde_json::from_str(body).ok()?;
    let rows = v.get("data")?.as_array()?;
    let prices: Vec<f64> = rows.iter()
        .filter_map(|row| row.get(4)?.as_f64())
        .collect();
    if prices.is_empty() { None } else { Some(prices) }
}
```

How it works:
- `serde_json::from_str(body).ok()?` — parse JSON; `?` returns `None` on any parse error
- `.get("data")?.as_array()?` — get the `"data"` key, assert it's an array; `?` on missing key or wrong type
- For each row (inner array), get element at index `[4]` and coerce to `f64`; `filter_map` skips missing/non-numeric fields
- Returns `None` if the resulting vec is empty (no valid prices extracted)

- [ ] **Step 2: Run tests — parse tests now pass**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/market-data-svc/tick-engine
cargo test 2>&1 | tail -15
```

Expected:
```
test tests::lmp_stays_in_range ... ok
test tests::deterministic_with_same_seed ... ok
test tests::parse_lmps_returns_none_on_bad_json ... ok
test tests::replay_uses_ou_when_empty ... ok
test tests::parse_lmps_extracts_prices ... ok
test tests::replay_cycles_buffer ... FAILED

test result: FAILED. 5 passed; 1 failed
```

`replay_cycles_buffer` still fails because `next()` hasn't been updated to use the buffer yet.

- [ ] **Step 3: Commit**

```bash
git add market-data-svc/tick-engine/src/lib.rs
git commit -m "feat(tick-engine): implement parse_ercot_json — extracts LMP prices from DAM SPP JSON"
git push origin main
```

---

## Task 4: Implement fetch + update `TickGenerator::next` + update `tick_generator_create`

**Files:**
- Modify: `market-data-svc/tick-engine/src/lib.rs`

This task adds `fetch_ercot_lmps`, updates `next()` to use the ERCOT buffer when non-empty, and updates `tick_generator_create` to call the fetch and log. After this, all 6 tests pass.

- [ ] **Step 1: Replace `market-data-svc/tick-engine/src/lib.rs` with the final implementation**

```rust
use rand::SeedableRng;
use rand_distr::{Distribution, Normal};

const THETA: f64     = 0.1;
const BASE_LOAD: f64 = 15_000.0;
const LOAD_VOL: f64  = 500.0;
const LMP_MIN: f64   = 1.0;
const LMP_MAX: f64   = 499.0;
const LOAD_MIN: f64  = 5_001.0;

pub struct TickGenerator {
    lmps:       Vec<f64>,
    idx:        usize,
    base_lmp:   f64,
    volatility: f64,
    lmp:        f64,
    rng:        rand::rngs::SmallRng,
    dist:       Normal<f64>,
}

impl TickGenerator {
    pub fn new(base_lmp: f64, volatility: f64, seed: u32) -> Self {
        Self {
            lmps:       vec![],
            idx:        0,
            base_lmp,
            volatility,
            lmp:        base_lmp,
            rng:        rand::rngs::SmallRng::seed_from_u64(seed as u64),
            dist:       Normal::new(0.0, 1.0).unwrap(),
        }
    }

    pub fn next(&mut self, out_lmp: &mut f64, out_load_mw: &mut f64) {
        if !self.lmps.is_empty() {
            *out_lmp = self.lmps[self.idx % self.lmps.len()];
            self.idx += 1;
        } else {
            self.lmp += THETA * (self.base_lmp - self.lmp)
                + self.volatility * self.dist.sample(&mut self.rng);
            self.lmp = self.lmp.clamp(LMP_MIN, LMP_MAX);
            *out_lmp = self.lmp;
        }
        let load = BASE_LOAD + LOAD_VOL * self.dist.sample(&mut self.rng);
        *out_load_mw = load.max(LOAD_MIN);
    }
}

fn parse_ercot_json(body: &str) -> Option<Vec<f64>> {
    let v: serde_json::Value = serde_json::from_str(body).ok()?;
    let rows = v.get("data")?.as_array()?;
    let prices: Vec<f64> = rows.iter()
        .filter_map(|row| row.get(4)?.as_f64())
        .collect();
    if prices.is_empty() { None } else { Some(prices) }
}

fn fetch_ercot_lmps(settlement_point: &str, date: &str) -> Option<Vec<f64>> {
    let url = format!(
        "https://api.ercot.com/api/public-reports/np4-190-cd/dam_stlmnt_pnt_prices\
         ?deliveryDateFrom={date}&deliveryDateTo={date}\
         &settlementPoint={settlement_point}&size=96"
    );
    let body = ureq::get(&url)
        .call()
        .ok()?
        .into_string()
        .ok()?;
    parse_ercot_json(&body)
}

// --- extern "C" ABI — same symbols as the former C++ libmarketdata.so ---

#[no_mangle]
pub extern "C" fn tick_generator_create(
    base_lmp: f64,
    volatility: f64,
    seed: u32,
) -> *mut TickGenerator {
    let node = std::env::var("NODE_NAME")
        .unwrap_or_else(|_| "HB_NORTH".to_string());
    let date = std::env::var("ERCOT_REPLAY_DATE")
        .unwrap_or_else(|_| "2024-01-15".to_string());

    let lmps = match fetch_ercot_lmps(&node, &date) {
        Some(v) => {
            eprintln!("tick-engine: loaded {} ERCOT LMPs for {} ({})", v.len(), node, date);
            v
        }
        None => {
            eprintln!("tick-engine: ERCOT fetch failed for {}, using OU fallback", node);
            vec![]
        }
    };

    Box::into_raw(Box::new(TickGenerator {
        lmps,
        idx:        0,
        base_lmp,
        volatility,
        lmp:        base_lmp,
        rng:        rand::rngs::SmallRng::seed_from_u64(seed as u64),
        dist:       Normal::new(0.0, 1.0).unwrap(),
    }))
}

#[no_mangle]
pub extern "C" fn tick_generator_destroy(gen: *mut TickGenerator) {
    if !gen.is_null() {
        unsafe { drop(Box::from_raw(gen)); }
    }
}

#[no_mangle]
pub extern "C" fn tick_generator_next(
    gen: *mut TickGenerator,
    out_lmp: *mut f64,
    out_load_mw: *mut f64,
) {
    if gen.is_null() || out_lmp.is_null() || out_load_mw.is_null() {
        return;
    }
    unsafe { (*gen).next(&mut *out_lmp, &mut *out_load_mw); }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lmp_stays_in_range() {
        let mut g = TickGenerator::new(45.0, 5.0, 42);
        for _ in 0..1000 {
            let (mut lmp, mut load) = (0.0f64, 0.0f64);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0, "lmp={lmp} out of [1,499]");
            assert!(load > 5000.0, "load={load} not > 5000");
        }
    }

    #[test]
    fn deterministic_with_same_seed() {
        let mut g1 = TickGenerator::new(45.0, 5.0, 42);
        let mut g2 = TickGenerator::new(45.0, 5.0, 42);
        for i in 0..20 {
            let (mut l1, mut d1, mut l2, mut d2) = (0.0f64, 0.0f64, 0.0f64, 0.0f64);
            g1.next(&mut l1, &mut d1);
            g2.next(&mut l2, &mut d2);
            assert_eq!(l1, l2, "lmp diverged at tick {i}");
        }
    }

    #[test]
    fn parse_lmps_extracts_prices() {
        let json = r#"{"data":[["2024-01-15",1,1,"HB_NORTH",23.45],["2024-01-15",1,2,"HB_NORTH",24.10],["2024-01-15",1,3,"HB_NORTH",22.80]]}"#;
        let result = parse_ercot_json(json);
        assert_eq!(result, Some(vec![23.45, 24.10, 22.80]));
    }

    #[test]
    fn parse_lmps_returns_none_on_bad_json() {
        assert_eq!(parse_ercot_json("not json"), None);
    }

    #[test]
    fn replay_cycles_buffer() {
        let mut g = TickGenerator {
            lmps:       vec![10.0, 20.0, 30.0],
            idx:        0,
            base_lmp:   45.0,
            volatility: 5.0,
            lmp:        45.0,
            rng:        rand::rngs::SmallRng::seed_from_u64(42),
            dist:       Normal::new(0.0, 1.0).unwrap(),
        };
        let (mut lmp, mut load) = (0.0f64, 0.0f64);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 10.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 20.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 30.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 10.0, "should wrap around");
    }

    #[test]
    fn replay_uses_ou_when_empty() {
        let mut g = TickGenerator {
            lmps:       vec![],
            idx:        0,
            base_lmp:   45.0,
            volatility: 5.0,
            lmp:        45.0,
            rng:        rand::rngs::SmallRng::seed_from_u64(42),
            dist:       Normal::new(0.0, 1.0).unwrap(),
        };
        for _ in 0..100 {
            let (mut lmp, mut load) = (0.0f64, 0.0f64);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0, "lmp={lmp} out of [1,499]");
            assert!(load > 5000.0, "load={load} not > 5000");
        }
    }
}
```

Note: `TickGenerator::new()` still has `lmps: vec![]`, so the two existing tests (`lmp_stays_in_range`, `deterministic_with_same_seed`) always exercise the OU path — they are unaffected by the ERCOT logic.

- [ ] **Step 2: Run all 6 tests — all must pass**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/market-data-svc/tick-engine
cargo test 2>&1 | tail -15
```

Expected:
```
test tests::deterministic_with_same_seed ... ok
test tests::lmp_stays_in_range ... ok
test tests::parse_lmps_extracts_prices ... ok
test tests::parse_lmps_returns_none_on_bad_json ... ok
test tests::replay_cycles_buffer ... ok
test tests::replay_uses_ou_when_empty ... ok

test result: ok. 6 passed; 0 failed
```

- [ ] **Step 3: Commit**

```bash
git add market-data-svc/tick-engine/src/lib.rs
git commit -m "feat(tick-engine): ERCOT replay buffer — fetch DAM SPP, cycle on next, OU fallback"
git push origin main
```

---

## Task 5: Add `ERCOT_REPLAY_DATE` to `docker-compose.yml`

**Files:**
- Modify: `infra/docker-compose.yml`

Add `ERCOT_REPLAY_DATE: "2024-01-15"` to the `environment` block of all three `market-data-svc` services. `NODE_NAME` is already set on each service and maps directly to the ERCOT settlement point (`HB_NORTH`, `HB_SOUTH`, `HB_WEST`).

- [ ] **Step 1: Add `ERCOT_REPLAY_DATE` to `market-data-svc` (HB_NORTH)**

In `infra/docker-compose.yml`, find the `market-data-svc` service environment block (around line 20) and add the new variable:

```yaml
  market-data-svc:
    build:
      context: ../market-data-svc
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_NORTH
      BASE_LMP: "45.0"
      VOLATILITY: "5.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
      ERCOT_REPLAY_DATE: "2024-01-15"
    ports: ["9101:9101"]
    depends_on:
      postgres:
        condition: service_healthy
```

- [ ] **Step 2: Add `ERCOT_REPLAY_DATE` to `market-data-svc-south` (HB_SOUTH)**

Find `market-data-svc-south` environment block and add:

```yaml
  market-data-svc-south:
    build:
      context: ../market-data-svc
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_SOUTH
      BASE_LMP: "42.0"
      VOLATILITY: "7.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
      ERCOT_REPLAY_DATE: "2024-01-15"
    depends_on:
      postgres:
        condition: service_healthy
```

- [ ] **Step 3: Add `ERCOT_REPLAY_DATE` to `market-data-svc-west` (HB_WEST)**

Find `market-data-svc-west` environment block and add:

```yaml
  market-data-svc-west:
    build:
      context: ../market-data-svc
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_WEST
      BASE_LMP: "38.0"
      VOLATILITY: "6.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
      ERCOT_REPLAY_DATE: "2024-01-15"
    depends_on:
      postgres:
        condition: service_healthy
```

- [ ] **Step 4: Validate YAML**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose config --quiet
```

Expected: no output (silent success).

- [ ] **Step 5: Commit**

```bash
git add infra/docker-compose.yml
git commit -m "feat(infra): add ERCOT_REPLAY_DATE env var to market-data-svc services"
git push origin main
```

---

## Task 6: Build and verify ERCOT data flowing

**Files:** none — integration verification only.

The market-data-svc containers need to be rebuilt (new Rust code). The ERCOT fetch happens at container startup inside `tick_generator_create`. Stderr output from the Rust cdylib appears in `docker compose logs`.

- [ ] **Step 1: Rebuild and restart the market-data-svc containers**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose up -d --build market-data-svc market-data-svc-south market-data-svc-west
```

The Rust build inside Docker takes ~60–90s (Corrosion + ureq). Watch progress:

```bash
docker compose logs --follow market-data-svc 2>&1 | head -10
```

Press Ctrl-C once you see ticks flowing.

- [ ] **Step 2: Check logs for ERCOT load confirmation**

```bash
docker compose logs market-data-svc 2>&1 | grep "tick-engine"
docker compose logs market-data-svc-south 2>&1 | grep "tick-engine"
docker compose logs market-data-svc-west 2>&1 | grep "tick-engine"
```

Expected (one line per service):
```
tick-engine: loaded 96 ERCOT LMPs for HB_NORTH (2024-01-15)
tick-engine: loaded 96 ERCOT LMPs for HB_SOUTH (2024-01-15)
tick-engine: loaded 96 ERCOT LMPs for HB_WEST (2024-01-15)
```

If you see `ERCOT fetch failed for HB_NORTH, using OU fallback`:
1. Check network connectivity from Docker: `docker compose exec market-data-svc curl -s "https://api.ercot.com" | head -5`
2. If the endpoint URL has changed, check `https://api.ercot.com/api/public-reports/` for the current DAM SPP path
3. If the API is down, the OU fallback keeps ticks flowing — verify ticks still appear in the next step

- [ ] **Step 3: Verify ERCOT LMP values appearing in price_ticks table**

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "SELECT node, lmp, timestamp FROM price_ticks ORDER BY timestamp DESC LIMIT 6;"
```

When running on ERCOT data, LMP values will match real DAM prices (typically \$20–\$60/MWh for these hubs in January 2024) rather than the simulated OU range. Expected output:

```
   node   |   lmp    |          timestamp
----------+----------+----------------------------
 HB_NORTH | 28.3200  | 2026-05-17 ...
 HB_SOUTH | 25.1100  | 2026-05-17 ...
 HB_WEST  | 22.9500  | 2026-05-17 ...
 HB_NORTH | 28.3200  | 2026-05-17 ...
 ...
```

(The same 96 values repeat in a cycle every 96 ticks.)

- [ ] **Step 4: Verify spread-arb strategy is still firing**

The strategy-engine and spread-arb depend on LMP values, not their source. Check signals are still flowing:

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "SELECT strategy, node, side, created_at FROM signals ORDER BY created_at DESC LIMIT 6;"
```

Expected: recent rows for `mean_reversion`, `ma_crossover`, and `spread_arb`.

- [ ] **Step 5: Tag v0.7.0**

```bash
git tag v0.7.0
git push origin v0.7.0
```
