# Rust Migration: libmarketdata.so — System Design

**Date:** 2026-05-16
**Parent system:** trade_infra
**Scope:** Replace `libmarketdata.so` (C++ TickGenerator) with an identical-ABI Rust cdylib

---

## 1. Goal

Swap the C++ `libmarketdata.so` shared library for a Rust `cdylib` that exposes the same three `extern "C"` symbols with identical signatures. Zero changes to any consumer: `main.cpp`, `db_writer.cpp`, `metrics_server.cpp`, the Dockerfile runtime stage, and all three market-data-svc Docker Compose instances.

---

## 2. ABI Contract (unchanged)

`market-data-svc/include/marketdata.h` stays as-is:

```c
typedef struct TickGenerator TickGenerator;

TickGenerator* tick_generator_create(double base_lmp, double volatility, unsigned int seed);
void           tick_generator_destroy(TickGenerator* gen);
void           tick_generator_next(TickGenerator* gen, double* out_lmp, double* out_load_mw);
```

---

## 3. New Files

```
market-data-svc/
└── tick-engine/
    ├── Cargo.toml
    ├── src/lib.rs
    └── .cargo/config.toml      macOS linker flags
```

### tick-engine/Cargo.toml

```toml
[package]
name = "tick-engine"
version = "0.1.0"
edition = "2021"

[lib]
name = "marketdata"          # produces libmarketdata.so
crate-type = ["cdylib"]

[dependencies]
rand        = "0.8"
rand_distr  = "0.4"
```

### tick-engine/src/lib.rs

Ornstein-Uhlenbeck tick generator with `#[no_mangle] extern "C"` symbols:

```rust
use rand::SeedableRng;
use rand_distr::{Distribution, Normal};

const THETA: f64    = 0.1;
const BASE_LOAD: f64 = 15000.0;
const LOAD_VOL: f64  = 500.0;
const LMP_MIN: f64   = 1.0;
const LMP_MAX: f64   = 499.0;
const LOAD_MIN: f64  = 5001.0;

pub struct TickGenerator {
    base_lmp:   f64,
    volatility: f64,
    lmp:        f64,
    rng:        rand::rngs::SmallRng,
    dist:       Normal<f64>,
}

impl TickGenerator {
    pub fn new(base_lmp: f64, volatility: f64, seed: u32) -> Self {
        Self {
            base_lmp,
            volatility,
            lmp: base_lmp,
            rng: rand::rngs::SmallRng::seed_from_u64(seed as u64),
            dist: Normal::new(0.0, 1.0).unwrap(),
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

#[no_mangle]
pub extern "C" fn tick_generator_create(
    base_lmp: f64, volatility: f64, seed: u32,
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
    unsafe { (*gen).next(&mut *out_lmp, &mut *out_load_mw); }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lmp_stays_in_range() {
        let mut g = TickGenerator::new(45.0, 5.0, 42);
        for _ in 0..1000 {
            let (mut lmp, mut load) = (0.0, 0.0);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0);
            assert!(load > 5000.0);
        }
    }

    #[test]
    fn deterministic_with_same_seed() {
        let mut g1 = TickGenerator::new(45.0, 5.0, 42);
        let mut g2 = TickGenerator::new(45.0, 5.0, 42);
        for _ in 0..20 {
            let (mut l1, mut d1, mut l2, mut d2) = (0.0, 0.0, 0.0, 0.0);
            g1.next(&mut l1, &mut d1);
            g2.next(&mut l2, &mut d2);
            assert_eq!(l1, l2);
        }
    }
}
```

### tick-engine/.cargo/config.toml

```toml
[target.aarch64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]

[target.x86_64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]
```

---

## 4. Modified Files

### market-data-svc/CMakeLists.txt

Replace the C++ `marketdata` library target with Corrosion:

**Remove:**
```cmake
add_library(marketdata SHARED src/tick_generator.cpp)
target_include_directories(marketdata PUBLIC include PRIVATE src)
if(APPLE)
    target_link_options(marketdata PRIVATE -undefined dynamic_lookup)
endif()
```

**Add (before `find_package(PostgreSQL)`):**
```cmake
include(FetchContent)
FetchContent_Declare(corrosion
    GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
    GIT_TAG        v0.5.0)
FetchContent_MakeAvailable(corrosion)
corrosion_import_crate(MANIFEST_PATH tick-engine/Cargo.toml)
```

All other `target_link_libraries(... marketdata ...)` lines are unchanged — Corrosion creates a CMake target named `marketdata` matching the `[lib] name` in Cargo.toml.

### market-data-svc/Dockerfile (builder stage only)

```dockerfile
# BEFORE:
FROM ubuntu:22.04 AS builder
RUN apt-get update && apt-get install -y cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# AFTER:
FROM rust:latest AS builder
RUN apt-get update && apt-get install -y cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*
```

Runtime stage is **unchanged** — still copies `libmarketdata.so` from the same path.

---

## 5. Deleted Files

- `market-data-svc/src/tick_generator.h`
- `market-data-svc/src/tick_generator.cpp`

`include/marketdata.h` is **kept** — it is the ABI contract and may be used by future consumers or documentation.

---

## 6. Testing

### Rust unit tests

`cargo test` inside `tick-engine/` — 2 tests: `lmp_stays_in_range`, `deterministic_with_same_seed`.

### Existing GoogleTest suite (unchanged)

All 5 C++ tests run against the Rust `.so` via the same `extern "C"` ABI:

| Test | Verifies |
|---|---|
| `CreateAndDestroy` | Allocation/deallocation via Box |
| `LMPIsPositive` | LMP > 0 after first tick |
| `LMPStaysInRange` | Clamp [1.0, 499.0] over 1000 ticks |
| `LoadMWInRange` | load_mw > 5000 |
| `DeterministicWithSameSeed` | Same Rust seed → same output sequence |

Run: `cmake --build build --target test_marketdata && cd build && ctest --output-on-failure`

### Docker verification

`docker compose build market-data-svc` rebuilds with Rust cdylib. All 3 market-data instances (`HB_NORTH`, `HB_SOUTH`, `HB_WEST`) produce ticks as before.

---

## 7. Build Order

| Step | Action |
|---|---|
| 1 | Create `tick-engine/` with Cargo.toml, src/lib.rs, .cargo/config.toml |
| 2 | Write Rust unit tests, run `cargo test` (pass) |
| 3 | Update CMakeLists.txt — remove C++ target, add Corrosion |
| 4 | Build: `cmake -B build -S . && cmake --build build --target test_marketdata` |
| 5 | Run GoogleTest: `ctest --output-on-failure` — all 5 pass |
| 6 | Build main binary: `cmake --build build --target market_data_svc` |
| 7 | Delete `src/tick_generator.h` and `src/tick_generator.cpp` |
| 8 | Update Dockerfile builder stage |
| 9 | `docker compose build market-data-svc && docker compose up -d` — verify ticks |

---

## 8. What Does Not Change

- `include/marketdata.h` (ABI contract)
- `src/main.cpp`, `src/db_writer.cpp`, `src/metrics_server.cpp`
- Dockerfile runtime stage
- `infra/docker-compose.yml`
- `infra/prometheus.yml`
- All other services (order-svc, risk-svc, strategy-engine, Python strategies)
- GoogleTest test file (`tests/test_tick_generator.cpp`)
