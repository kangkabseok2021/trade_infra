# Rust Migration: libmarketdata.so Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the C++ `libmarketdata.so` shared library with an identical-ABI Rust cdylib, with zero changes to any consumer.

**Architecture:** A new `tick-engine/` Rust crate inside `market-data-svc/` is built as a `cdylib` named `marketdata` via Corrosion in CMakeLists.txt. The same three `extern "C"` symbols are exposed with `#[no_mangle]`. The existing 5 GoogleTest cases validate the Rust `.so` through the unchanged ABI. `src/tick_generator.cpp` and `src/tick_generator.h` are deleted after the tests pass.

**Tech Stack:** Rust 2021 + rand 0.8 + rand_distr 0.4, Corrosion v0.5.0 (FetchContent), CMake 3.20, GoogleTest 1.14, Docker (rust:latest builder).

---

## File Map

```
market-data-svc/
├── tick-engine/                   CREATE
│   ├── Cargo.toml                 CREATE — cdylib, name="marketdata"
│   ├── .cargo/config.toml         CREATE — macOS linker flags
│   └── src/lib.rs                 CREATE — OU process + extern "C"
├── CMakeLists.txt                 MODIFY — remove C++ target, add Corrosion
├── Dockerfile                     MODIFY — builder base: rust:latest
├── src/tick_generator.h           DELETE (Task 7)
└── src/tick_generator.cpp         DELETE (Task 7)
```

---

### Task 1: Scaffold tick-engine/ crate

**Files:**
- Create: `market-data-svc/tick-engine/Cargo.toml`
- Create: `market-data-svc/tick-engine/.cargo/config.toml`
- Create: `market-data-svc/tick-engine/src/lib.rs` (empty placeholder)

- [ ] **Step 1: Create directory and placeholder lib.rs**

```bash
mkdir -p market-data-svc/tick-engine/src
touch market-data-svc/tick-engine/src/lib.rs
```

- [ ] **Step 2: Write Cargo.toml**

Create `market-data-svc/tick-engine/Cargo.toml`:

```toml
[package]
name = "tick-engine"
version = "0.1.0"
edition = "2021"

[lib]
name = "marketdata"
crate-type = ["cdylib"]

[dependencies]
rand       = "0.8"
rand_distr = "0.4"
```

The `name = "marketdata"` in `[lib]` makes Cargo produce `libmarketdata.so` / `libmarketdata.dylib`. Corrosion will expose a CMake target with the same name, so existing `target_link_libraries(... marketdata ...)` lines work unchanged.

- [ ] **Step 3: Write .cargo/config.toml (macOS linker flags)**

Create `market-data-svc/tick-engine/.cargo/config.toml`:

```toml
[target.aarch64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]

[target.x86_64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]
```

This is the Rust equivalent of the C++ `-undefined dynamic_lookup` linker flag that was previously in CMakeLists.txt.

- [ ] **Step 4: Verify Cargo.toml parses**

```bash
cd market-data-svc/tick-engine && cargo check 2>&1 | head -5
```

Expected: no errors (empty lib is valid Rust).

- [ ] **Step 5: Commit**

```bash
git add market-data-svc/tick-engine/
git commit -m "feat(tick-engine): scaffold Rust cdylib crate — libmarketdata.so"
git push origin main
```

---

### Task 2: Write failing Rust unit tests

**Files:**
- Modify: `market-data-svc/tick-engine/src/lib.rs`

- [ ] **Step 1: Write failing tests into lib.rs**

Replace the empty `market-data-svc/tick-engine/src/lib.rs` with:

```rust
// Implementation will be added in Task 3.

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
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd market-data-svc/tick-engine && cargo test 2>&1 | head -10
```

Expected: `error[E0422]: cannot find struct, variant or union type 'TickGenerator'`

- [ ] **Step 3: Commit failing tests**

```bash
git add market-data-svc/tick-engine/src/lib.rs
git commit -m "test(tick-engine): Rust unit tests for TickGenerator (failing)"
git push origin main
```

---

### Task 3: Implement TickGenerator — pass Rust tests

**Files:**
- Modify: `market-data-svc/tick-engine/src/lib.rs`

- [ ] **Step 1: Write full lib.rs implementation**

Replace `market-data-svc/tick-engine/src/lib.rs` with:

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
}
```

- [ ] **Step 2: Run Rust tests**

```bash
cd market-data-svc/tick-engine && cargo test
```

Expected:
```
running 2 tests
test tests::deterministic_with_same_seed ... ok
test tests::lmp_stays_in_range ... ok
test result: ok. 2 passed; 0 failed
```

- [ ] **Step 3: Confirm cdylib builds**

```bash
cd market-data-svc/tick-engine && cargo build --release 2>&1 | tail -3
ls target/release/libmarketdata.*
```

Expected: `libmarketdata.dylib` (macOS) or `libmarketdata.so` (Linux).

- [ ] **Step 4: Commit**

```bash
git add market-data-svc/tick-engine/src/lib.rs
git commit -m "feat(tick-engine): implement Rust OU TickGenerator — 2/2 tests pass"
git push origin main
```

---

### Task 4: Update CMakeLists.txt — swap C++ target for Corrosion

**Files:**
- Modify: `market-data-svc/CMakeLists.txt`

- [ ] **Step 1: Replace CMakeLists.txt**

Overwrite `market-data-svc/CMakeLists.txt` with:

```cmake
cmake_minimum_required(VERSION 3.20)
project(market_data_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)

# Corrosion: build Rust tick-engine cdylib as CMake target "marketdata"
FetchContent_Declare(corrosion
    GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
    GIT_TAG        v0.5.0)
FetchContent_MakeAvailable(corrosion)
corrosion_import_crate(MANIFEST_PATH tick-engine/Cargo.toml)

FetchContent_Declare(httplib
    GIT_REPOSITORY https://github.com/yhirose/cpp-httplib.git
    GIT_TAG        v0.15.3)
FetchContent_MakeAvailable(httplib)

find_package(PostgreSQL REQUIRED)

# Main service binary — unchanged
add_executable(market_data_svc
    src/main.cpp src/db_writer.cpp src/metrics_server.cpp)
target_include_directories(market_data_svc PRIVATE include src)
target_link_libraries(market_data_svc marketdata httplib::httplib PostgreSQL::PostgreSQL)

option(BUILD_TESTS "Build test suite" ON)
if(BUILD_TESTS)
    FetchContent_Declare(googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG        v1.14.0)
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)

    enable_testing()
    add_executable(test_marketdata tests/test_tick_generator.cpp)
    target_link_libraries(test_marketdata marketdata GTest::gtest_main)
    include(GoogleTest)
    gtest_discover_tests(test_marketdata)
endif()
```

Key changes from the original:
- **Removed:** `add_library(marketdata SHARED src/tick_generator.cpp)` and its `target_include_directories` and `if(APPLE) target_link_options` block
- **Added:** Corrosion FetchContent block + `corrosion_import_crate(...)` which creates the `marketdata` CMake target from the Rust crate

- [ ] **Step 2: Commit CMakeLists.txt**

```bash
git add market-data-svc/CMakeLists.txt
git commit -m "build(market-data-svc): replace C++ marketdata target with Corrosion Rust cdylib"
git push origin main
```

---

### Task 5: Build and run all 5 GoogleTest cases against Rust .so

**Files:** none created — build verification only

- [ ] **Step 1: Wipe stale build cache and reconfigure**

The old CMakeCache references the C++ sources. Delete it first:

```bash
rm -rf market-data-svc/build
cmake -B market-data-svc/build -S market-data-svc \
      -DCMAKE_EXPORT_COMPILE_COMMANDS=ON 2>&1 | tail -8
```

Expected output includes:
```
-- Corrosion: Found Rust toolchain ...
-- Configuring done
-- Build files have been written to: .../market-data-svc/build
```

If `rustup` is not found: install with `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`.

- [ ] **Step 2: Build test_marketdata (triggers Rust build automatically)**

```bash
cmake --build market-data-svc/build --target test_marketdata -j4 2>&1 | tail -8
```

Expected: Cargo builds `libmarketdata.dylib` then C++ links `test_marketdata`.

- [ ] **Step 3: Run all 5 GoogleTest cases**

```bash
cd market-data-svc/build && ctest --output-on-failure
```

Expected:
```
1/5 Test #1: TickGeneratorTest.CreateAndDestroy ............   Passed
2/5 Test #2: TickGeneratorTest.LMPIsPositive ...............   Passed
3/5 Test #3: TickGeneratorTest.LMPStaysInRange .............   Passed
4/5 Test #4: TickGeneratorTest.LoadMWInRange ...............   Passed
5/5 Test #5: TickGeneratorTest.DeterministicWithSameSeed ...   Passed
100% tests passed, 0 tests failed out of 5
```

If any test fails: check that the Rust clamp [1.0, 499.0] and load floor 5001.0 match the GoogleTest assertions in `tests/test_tick_generator.cpp`.

- [ ] **Step 4: No commit needed — verification only**

---

### Task 6: Build market_data_svc binary

**Files:** none — build verification only

- [ ] **Step 1: Build the main binary**

```bash
cmake --build market-data-svc/build --target market_data_svc -j4 2>&1 | tail -5
```

Expected: `[100%] Linking CXX executable market_data_svc`

- [ ] **Step 2: Verify the binary loads the Rust library**

```bash
otool -L market-data-svc/build/market_data_svc | grep marketdata
```

Expected: a line referencing `libmarketdata.dylib`.

- [ ] **Step 3: Quick smoke test (requires local PostgreSQL)**

```bash
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra" \
NODE_NAME=HB_NORTH BASE_LMP=45.0 VOLATILITY=5.0 INTERVAL_MS=200 \
market-data-svc/build/market_data_svc &
PID=$!
sleep 3
psql trade_infra -c "SELECT COUNT(*) FROM price_ticks WHERE node='HB_NORTH'"
kill $PID
```

Expected: count ≥ 10 ticks.

---

### Task 7: Delete C++ implementation files

**Files:**
- Delete: `market-data-svc/src/tick_generator.h`
- Delete: `market-data-svc/src/tick_generator.cpp`

- [ ] **Step 1: Remove the C++ files**

```bash
git rm market-data-svc/src/tick_generator.h market-data-svc/src/tick_generator.cpp
```

- [ ] **Step 2: Rebuild to confirm nothing is broken**

```bash
cmake --build market-data-svc/build --target market_data_svc test_marketdata -j4 2>&1 | tail -3
```

Expected: builds cleanly (CMake no longer references these files).

- [ ] **Step 3: Re-run tests**

```bash
cd market-data-svc/build && ctest --output-on-failure 2>&1 | tail -3
```

Expected: `100% tests passed, 0 tests failed out of 5`

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(market-data-svc): delete C++ TickGenerator — Rust cdylib is the sole implementation"
git push origin main
```

---

### Task 8: Update Dockerfile builder stage

**Files:**
- Modify: `market-data-svc/Dockerfile`

- [ ] **Step 1: Replace builder base image**

In `market-data-svc/Dockerfile`, replace only the first line of the builder stage:

**Before:**
```dockerfile
FROM ubuntu:22.04 AS builder
RUN apt-get update && apt-get install -y \
    cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*
```

**After:**
```dockerfile
FROM rust:latest AS builder
RUN apt-get update && apt-get install -y \
    cmake build-essential git libpq-dev \
    && rm -rf /var/lib/apt/lists/*
```

The `rust:latest` image is Debian-based and includes `cargo` and `rustup`. All other Dockerfile lines are **unchanged** — the runtime stage still copies `libmarketdata.so` from the same build path.

- [ ] **Step 2: Verify Docker build succeeds**

```bash
cd market-data-svc && docker build --target builder -t test-rust-builder . 2>&1 | tail -10
```

Expected: builder stage completes, Cargo builds libmarketdata.so, CMake links market_data_svc.

- [ ] **Step 3: Commit**

```bash
git add market-data-svc/Dockerfile
git commit -m "build(market-data-svc): use rust:latest as Docker builder for Corrosion"
git push origin main
```

---

### Task 9: docker compose build and verify all 3 nodes

**Files:** none — final integration verification

- [ ] **Step 1: Rebuild all three market-data-svc images**

```bash
cd infra && docker compose build market-data-svc market-data-svc-south market-data-svc-west 2>&1 | tail -8
```

Expected:
```
Image infra-market-data-svc Built
Image infra-market-data-svc-south Built
Image infra-market-data-svc-west Built
```

- [ ] **Step 2: Restart the stack**

```bash
docker compose up -d --force-recreate market-data-svc market-data-svc-south market-data-svc-west
```

- [ ] **Step 3: Wait for ticks from all 3 nodes**

```bash
until docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT COUNT(DISTINCT node) FROM price_ticks" \
  2>/dev/null | grep -q " 3"; do sleep 3; done && echo "All 3 nodes ticking"
```

- [ ] **Step 4: Verify tick counts**

```bash
docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT node, COUNT(*) FROM price_ticks GROUP BY node ORDER BY node"
```

Expected: rows for HB_NORTH, HB_SOUTH, HB_WEST each with 10+ ticks.

- [ ] **Step 5: Tag release**

```bash
git tag -a v0.4.0 -m "v0.4.0 — Rust migration: libmarketdata.so

C++ TickGenerator replaced with Rust cdylib via Corrosion.
Same extern-C ABI, same 5 GoogleTest cases passing, zero consumer changes.
First production Rust component in trade_infra."
git push origin v0.4.0
```

---

## Spec Coverage Check

| Spec requirement | Task |
|---|---|
| Cargo.toml: crate-type=cdylib, name="marketdata" | Task 1 |
| .cargo/config.toml: macOS -undefined dynamic_lookup | Task 1 |
| Rust OU process: THETA=0.1, clamp [1,499], load floor 5001 | Task 3 |
| #[no_mangle] extern "C": create/destroy/next with correct signatures | Task 3 |
| SmallRng seeded from u32 seed | Task 3 |
| Rust unit tests: lmp_stays_in_range, deterministic_with_same_seed | Tasks 2–3 |
| CMakeLists.txt: remove C++ marketdata target | Task 4 |
| CMakeLists.txt: add Corrosion v0.5.0 FetchContent | Task 4 |
| corrosion_import_crate creates "marketdata" CMake target | Task 4 |
| All 5 GoogleTest cases pass against Rust .so | Task 5 |
| market_data_svc binary builds and links Rust library | Task 6 |
| src/tick_generator.h and .cpp deleted | Task 7 |
| Dockerfile builder: FROM rust:latest | Task 8 |
| docker compose: all 3 market-data instances produce ticks | Task 9 |
