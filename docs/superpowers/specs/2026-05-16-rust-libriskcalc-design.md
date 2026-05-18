# Rust Migration: libriskcalc.so — System Design

**Date:** 2026-05-16
**Parent system:** trade_infra
**Scope:** Replace `libriskcalc.so` (C++ RiskCalc) with an identical-ABI Rust cdylib

---

## 1. Goal

Swap the C++ `libriskcalc.so` shared library for a Rust `cdylib` that exposes the same three `extern "C"` symbols with identical signatures. Zero changes to any consumer: `riskcalc.go` (CGo wrapper), the Dockerfile Stages 2–3, and all risk-svc Go packages.

---

## 2. ABI Contract (unchanged)

`risk-svc/include/riskcalc.h` stays as-is:

```c
double calc_mtm_pnl(double net_mw, double avg_fill_price, double current_lmp);
double calc_net_exposure(double net_mw, double current_lmp);
int    check_limit_breach(double net_exposure_mw, double position_limit_mw);
```

---

## 3. New Files

```
risk-svc/
└── risk-calc-rs/
    ├── Cargo.toml
    ├── src/lib.rs
    └── .cargo/config.toml      macOS linker flags
```

### risk-calc-rs/Cargo.toml

```toml
[package]
name = "risk-calc-rs"
version = "0.1.0"
edition = "2021"

[lib]
name = "riskcalc"          # produces libriskcalc.so
crate-type = ["cdylib"]
```

No external dependencies — pure math on primitive types.

### risk-calc-rs/src/lib.rs

```rust
use std::os::raw::c_int;

#[no_mangle]
pub extern "C" fn calc_mtm_pnl(
    net_mw: f64,
    avg_fill_price: f64,
    current_lmp: f64,
) -> f64 {
    net_mw * (current_lmp - avg_fill_price)
}

#[no_mangle]
pub extern "C" fn calc_net_exposure(net_mw: f64, _current_lmp: f64) -> f64 {
    // _current_lmp reserved for future USD-denominated variant
    net_mw.abs()
}

#[no_mangle]
pub extern "C" fn check_limit_breach(
    net_exposure_mw: f64,
    position_limit_mw: f64,
) -> c_int {
    if net_exposure_mw > position_limit_mw { 1 } else { 0 }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mtm_pnl_long_profit() {
        // long 10 MW, bought at 40, current 45 → +50
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 45.0), 50.0);
    }

    #[test]
    fn mtm_pnl_long_loss() {
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 35.0), -50.0);
    }

    #[test]
    fn mtm_pnl_short() {
        // short -5 MW, avg 50, current 45 → -5*(45-50) = +25
        assert_eq!(calc_mtm_pnl(-5.0, 50.0, 45.0), 25.0);
    }

    #[test]
    fn net_exposure_positive() {
        assert_eq!(calc_net_exposure(10.0, 45.0), 10.0);
    }

    #[test]
    fn net_exposure_negative() {
        assert_eq!(calc_net_exposure(-7.0, 45.0), 7.0);
    }

    #[test]
    fn net_exposure_ignores_lmp() {
        assert_eq!(calc_net_exposure(5.0, 0.0), calc_net_exposure(5.0, 999.0));
    }

    #[test]
    fn limit_breach_over() {
        assert_eq!(check_limit_breach(51.0, 50.0), 1);
    }

    #[test]
    fn limit_breach_under() {
        assert_eq!(check_limit_breach(49.0, 50.0), 0);
    }

    #[test]
    fn limit_breach_exact() {
        // equal is not a breach (strict >)
        assert_eq!(check_limit_breach(50.0, 50.0), 0);
    }
}
```

### risk-calc-rs/.cargo/config.toml

```toml
[target.aarch64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]

[target.x86_64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]
```

---

## 4. Modified Files

### risk-svc/CMakeLists.txt

**Remove:**
```cmake
add_library(riskcalc SHARED src/risk_calc.cpp)
target_include_directories(riskcalc PUBLIC include PRIVATE src)
if(APPLE)
    target_link_options(riskcalc PRIVATE -undefined dynamic_lookup)
endif()
```

**Add (before `find_package(PostgreSQL)`):**
```cmake
include(FetchContent)
FetchContent_Declare(corrosion
    GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
    GIT_TAG        v0.5.0)
FetchContent_MakeAvailable(corrosion)
corrosion_import_crate(MANIFEST_PATH risk-calc-rs/Cargo.toml)
```

`target_link_libraries(test_riskcalc riskcalc GTest::gtest_main)` is **unchanged** — Corrosion creates a CMake target named `riskcalc` matching the `[lib] name` in Cargo.toml.

### risk-svc/Dockerfile

Only Stage 1 changes. Rename `cpp-builder` → `rust-builder` and swap base image:

```dockerfile
# Stage 1: BEFORE
FROM ubuntu:22.04 AS cpp-builder
RUN apt-get update && apt-get install -y cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Stage 1: AFTER
FROM rust:latest AS rust-builder
RUN apt-get update && apt-get install -y cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*
```

Add `RUSTC_REAL` workaround to cmake invocation:
```dockerfile
RUN RUSTC_REAL="$(rustup which rustc)" \
    && cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
        -DRust_COMPILER="${RUSTC_REAL}" \
    && cmake --build build --target risk_svc_bin -j"$(nproc)"
```

Stage 2 (`go-builder`) — update `COPY --from` alias only:
```dockerfile
COPY --from=rust-builder /build/build/libriskcalc.so /app/build/libriskcalc.so
COPY --from=rust-builder /build/include /app/include
```

Stage 3 (runtime) — **completely unchanged**.

---

## 5. Deleted Files

- `risk-svc/src/risk_calc.h`
- `risk-svc/src/risk_calc.cpp`

`include/riskcalc.h` is **kept** — it is the ABI contract and is `#include`d by the CGo wrapper.

---

## 6. Unchanged Files

- `risk-svc/include/riskcalc.h`
- `risk-svc/internal/riskcalc/riskcalc.go` (CGo wrapper — zero changes)
- All other risk-svc Go packages
- `infra/docker-compose.yml`
- `infra/prometheus.yml`
- All other services

---

## 7. Testing

### Rust unit tests

`cargo test` inside `risk-calc-rs/` — 9 tests covering all three functions including sign, direction, and edge cases.

### Existing GoogleTest suite (unchanged)

All 9 C++ tests run against the Rust `.so` via the same `extern "C"` ABI:

| Test | Verifies |
|---|---|
| `MtmPnlLongProfit` | Positive PnL for profitable long |
| `MtmPnlLongLoss` | Negative PnL for losing long |
| `MtmPnlShortProfit` | Positive PnL for profitable short |
| `NetExposurePositive` | abs(positive net_mw) |
| `NetExposureNegative` | abs(negative net_mw) |
| `NetExposureIgnoresLmp` | current_lmp has no effect |
| `LimitBreachOver` | Returns 1 when exposure > limit |
| `LimitBreachUnder` | Returns 0 when exposure < limit |
| `LimitBreachExact` | Returns 0 when exposure == limit (strict >) |

Run: `cmake --build build --target test_riskcalc && cd build && ctest --output-on-failure`

---

## 8. Build Order

| Step | Action |
|---|---|
| 1 | Create `risk-calc-rs/` with Cargo.toml, src/lib.rs, .cargo/config.toml |
| 2 | Write Rust unit tests, run `cargo test` (pass) |
| 3 | Update CMakeLists.txt — remove C++ target, add Corrosion |
| 4 | Build: `cmake -B build -S . && cmake --build build --target test_riskcalc` |
| 5 | Run GoogleTest: `ctest --output-on-failure` — all 9 pass |
| 6 | Build main binary: `cmake --build build --target risk_svc_bin` |
| 7 | Delete `src/risk_calc.h` and `src/risk_calc.cpp` |
| 8 | Update risk-svc/Dockerfile Stage 1 |
| 9 | `docker compose build risk-svc && docker compose up -d` — verify risk metrics flowing |
