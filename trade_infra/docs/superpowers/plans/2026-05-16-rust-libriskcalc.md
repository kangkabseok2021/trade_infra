# Rust Migration: libriskcalc.so Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `risk-svc/src/risk_calc.cpp` with a Rust cdylib that exposes the same three `extern "C"` symbols, leaving the CGo consumer (`riskcalc.go`) and all other files untouched.

**Architecture:** A new `risk-calc-rs/` Rust crate inside `risk-svc/` produces `libriskcalc.so` via Corrosion v0.5.0. CMakeLists.txt swaps the C++ `add_library(riskcalc …)` for `corrosion_import_crate`. The three functions are pure math on primitive types — no unsafe blocks, no dependencies. The 3-stage Dockerfile changes only Stage 1 (base image + RUSTC_REAL workaround) and the two `COPY --from` aliases in Stages 2 and 3.

**Tech Stack:** Rust 2021 edition (no external crates), Corrosion v0.5.0, CMake 3.20, GoogleTest v1.14.0, Docker, Go 1.26 (CGo — unchanged)

---

## File Map

| Action | Path | Purpose |
|--------|------|---------|
| Create | `risk-svc/risk-calc-rs/Cargo.toml` | Crate manifest; `[lib] name = "riskcalc"` produces `libriskcalc.so` |
| Create | `risk-svc/risk-calc-rs/src/lib.rs` | Three `#[no_mangle] extern "C"` functions; pure math, no unsafe |
| Create | `risk-svc/risk-calc-rs/.cargo/config.toml` | macOS linker flag (`-undefined dynamic_lookup`) |
| Modify | `risk-svc/CMakeLists.txt` | Swap C++ `add_library` for Corrosion; add `include` path to test target |
| Modify | `risk-svc/Dockerfile` | Stage 1: `rust:latest` + RUSTC_REAL; Stage 2 & 3: update `COPY --from` alias |
| Delete | `risk-svc/src/risk_calc.h` | Replaced by Rust |
| Delete | `risk-svc/src/risk_calc.cpp` | Replaced by Rust |
| Keep | `risk-svc/include/riskcalc.h` | ABI contract; still `#include`d by CGo wrapper |
| Keep | `risk-svc/internal/riskcalc/riskcalc.go` | Zero changes needed |

---

### Task 1: Scaffold the Rust crate

**Files:**
- Create: `risk-svc/risk-calc-rs/Cargo.toml`
- Create: `risk-svc/risk-calc-rs/.cargo/config.toml`
- Create: `risk-svc/risk-calc-rs/src/lib.rs` (empty stub)

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/risk-calc-rs/src
mkdir -p /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/risk-calc-rs/.cargo
```

- [ ] **Step 2: Write Cargo.toml**

Create `risk-svc/risk-calc-rs/Cargo.toml`:

```toml
[package]
name = "risk-calc-rs"
version = "0.1.0"
edition = "2021"

[lib]
name = "riskcalc"
crate-type = ["cdylib"]
```

No `[dependencies]` section — pure math, nothing needed.

- [ ] **Step 3: Write .cargo/config.toml** (macOS linker flags)

Create `risk-svc/risk-calc-rs/.cargo/config.toml`:

```toml
[target.aarch64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]

[target.x86_64-apple-darwin]
rustflags = ["-C", "link-arg=-undefined", "-C", "link-arg=dynamic_lookup"]
```

- [ ] **Step 4: Create empty lib.rs stub**

Create `risk-svc/risk-calc-rs/src/lib.rs`:

```rust
```

(Empty file — tests in Task 2 will fail to compile until Task 3 fills this in.)

- [ ] **Step 5: Verify cargo knows the crate**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/risk-calc-rs
cargo metadata --no-deps --format-version 1 | grep '"name":"riskcalc"'
```

Expected: line containing `"name":"riskcalc"` confirming the lib name.

- [ ] **Step 6: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git add risk-svc/risk-calc-rs/
git commit -m "chore(risk-svc): scaffold risk-calc-rs Rust crate"
git push origin main
```

---

### Task 2: Write failing Rust unit tests

**Files:**
- Modify: `risk-svc/risk-calc-rs/src/lib.rs` (add test module; functions not yet implemented)

- [ ] **Step 1: Write tests in lib.rs**

Replace `risk-svc/risk-calc-rs/src/lib.rs` with:

```rust
use std::os::raw::c_int;

#[no_mangle]
pub extern "C" fn calc_mtm_pnl(
    _net_mw: f64,
    _avg_fill_price: f64,
    _current_lmp: f64,
) -> f64 {
    unimplemented!()
}

#[no_mangle]
pub extern "C" fn calc_net_exposure(_net_mw: f64, _current_lmp: f64) -> f64 {
    unimplemented!()
}

#[no_mangle]
pub extern "C" fn check_limit_breach(
    _net_exposure_mw: f64,
    _position_limit_mw: f64,
) -> c_int {
    unimplemented!()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[should_panic]
    fn mtm_pnl_long_profit() {
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 45.0), 50.0);
    }

    #[test]
    #[should_panic]
    fn mtm_pnl_long_loss() {
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 35.0), -50.0);
    }

    #[test]
    #[should_panic]
    fn mtm_pnl_short() {
        assert_eq!(calc_mtm_pnl(-5.0, 50.0, 45.0), 25.0);
    }

    #[test]
    #[should_panic]
    fn net_exposure_positive() {
        assert_eq!(calc_net_exposure(10.0, 45.0), 10.0);
    }

    #[test]
    #[should_panic]
    fn net_exposure_negative() {
        assert_eq!(calc_net_exposure(-7.0, 45.0), 7.0);
    }

    #[test]
    #[should_panic]
    fn net_exposure_ignores_lmp() {
        assert_eq!(
            calc_net_exposure(5.0, 0.0),
            calc_net_exposure(5.0, 999.0),
        );
    }

    #[test]
    #[should_panic]
    fn limit_breach_over() {
        assert_eq!(check_limit_breach(51.0, 50.0), 1);
    }

    #[test]
    #[should_panic]
    fn limit_breach_under() {
        assert_eq!(check_limit_breach(49.0, 50.0), 0);
    }

    #[test]
    #[should_panic]
    fn limit_breach_exact() {
        assert_eq!(check_limit_breach(50.0, 50.0), 0);
    }
}
```

Note: `#[should_panic]` means each test passes when the stub panics. After implementation in Task 3 the stubs are replaced and `#[should_panic]` is removed.

- [ ] **Step 2: Run tests — verify they all pass (panics caught by should_panic)**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/risk-calc-rs
cargo test
```

Expected: `9 passed` (all should_panic tests pass because `unimplemented!()` panics).

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git add risk-svc/risk-calc-rs/src/lib.rs
git commit -m "test(risk-calc-rs): add 9 failing Rust unit tests"
git push origin main
```

---

### Task 3: Implement the three extern "C" functions

**Files:**
- Modify: `risk-svc/risk-calc-rs/src/lib.rs` (replace stubs; remove `#[should_panic]`)

- [ ] **Step 1: Write full implementation**

Replace `risk-svc/risk-calc-rs/src/lib.rs` with:

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
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 45.0), 50.0);
    }

    #[test]
    fn mtm_pnl_long_loss() {
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 35.0), -50.0);
    }

    #[test]
    fn mtm_pnl_short() {
        // short -5 MW, avg 50, current 45: -5 * (45-50) = +25
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
        assert_eq!(
            calc_net_exposure(5.0, 0.0),
            calc_net_exposure(5.0, 999.0),
        );
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
        // equal is not a breach — strict >
        assert_eq!(check_limit_breach(50.0, 50.0), 0);
    }
}
```

- [ ] **Step 2: Run Rust tests — verify 9 pass**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/risk-calc-rs
cargo test
```

Expected output:
```
running 9 tests
test tests::limit_breach_exact ... ok
test tests::limit_breach_over ... ok
test tests::limit_breach_under ... ok
test tests::mtm_pnl_long_loss ... ok
test tests::mtm_pnl_long_profit ... ok
test tests::mtm_pnl_short ... ok
test tests::net_exposure_ignores_lmp ... ok
test tests::net_exposure_negative ... ok
test tests::net_exposure_positive ... ok

test result: ok. 9 passed; 0 failed
```

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git add risk-svc/risk-calc-rs/src/lib.rs
git commit -m "feat(risk-calc-rs): implement calc_mtm_pnl, calc_net_exposure, check_limit_breach"
git push origin main
```

---

### Task 4: Update CMakeLists.txt — swap C++ target for Corrosion

**Files:**
- Modify: `risk-svc/CMakeLists.txt`

Current file for reference:
```cmake
cmake_minimum_required(VERSION 3.20)
project(risk_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_library(riskcalc SHARED src/risk_calc.cpp)
target_include_directories(riskcalc PUBLIC include PRIVATE src)
if(APPLE)
    target_link_options(riskcalc PRIVATE -undefined dynamic_lookup)
endif()

option(BUILD_TESTS "Build test suite" ON)
if(BUILD_TESTS)
    include(FetchContent)
    FetchContent_Declare(googletest ...)
    ...
    add_executable(test_riskcalc tests/test_risk_calc.cpp)
    target_link_libraries(test_riskcalc riskcalc GTest::gtest_main)
    ...
endif()
```

- [ ] **Step 1: Write updated CMakeLists.txt**

Replace `risk-svc/CMakeLists.txt` with:

```cmake
cmake_minimum_required(VERSION 3.20)
project(risk_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)
FetchContent_Declare(corrosion
    GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
    GIT_TAG        v0.5.0)
FetchContent_MakeAvailable(corrosion)
corrosion_import_crate(MANIFEST_PATH risk-calc-rs/Cargo.toml)

option(BUILD_TESTS "Build test suite" ON)
if(BUILD_TESTS)
    FetchContent_Declare(googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG        v1.14.0)
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)

    enable_testing()
    add_executable(test_riskcalc tests/test_risk_calc.cpp)
    target_include_directories(test_riskcalc PRIVATE ${CMAKE_SOURCE_DIR}/include)
    target_link_libraries(test_riskcalc riskcalc GTest::gtest_main)
    include(GoogleTest)
    gtest_discover_tests(test_riskcalc)
endif()
```

Key changes:
- Removed: `add_library(riskcalc SHARED src/risk_calc.cpp)` and its `target_include_directories` and `if(APPLE) target_link_options`
- Added: Corrosion FetchContent block before the tests block
- Added: `target_include_directories(test_riskcalc PRIVATE ${CMAKE_SOURCE_DIR}/include)` — Corrosion's imported target doesn't propagate the `include/` path, so the test binary needs it explicitly
- The `FetchContent_Declare(googletest …)` moved inside `if(BUILD_TESTS)` to share the top-level `include(FetchContent)` already added for Corrosion

- [ ] **Step 2: Delete stale build directory if it exists**

```bash
rm -rf /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/build
```

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git add risk-svc/CMakeLists.txt
git commit -m "build(risk-svc): swap C++ riskcalc target for Corrosion cdylib"
git push origin main
```

---

### Task 5: Build test_riskcalc and run all 9 GoogleTests

**Files:** None (verification only)

- [ ] **Step 1: Configure CMake with Rust compiler path**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc
RUSTC_REAL="$(rustup which rustc)"
cmake -B build -S . -DCMAKE_BUILD_TYPE=Debug -DRust_COMPILER="${RUSTC_REAL}"
```

Expected: CMake configuration succeeds. You will see Corrosion fetching from GitHub and output like:
```
-- Corrosion: Found rustc ...
-- Corrosion: importing crate risk-calc-rs
```

- [ ] **Step 2: Build test_riskcalc**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc
cmake --build build --target test_riskcalc -j"$(nproc)"
```

Expected: Rust library compiles first (you'll see `Compiling risk-calc-rs`), then the C++ test binary links against it.

- [ ] **Step 3: Run all 9 GoogleTests**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/build
ctest --output-on-failure
```

Expected:
```
Test project .../risk-svc/build
      Start 1: MtmPnlLongProfit
  1/9 Test #1: MtmPnlLongProfit ................   Passed
      Start 2: MtmPnlLongLoss
  2/9 Test #2: MtmPnlLongLoss ..................   Passed
      Start 3: MtmPnlShortProfit
  3/9 Test #3: MtmPnlShortProfit ...............   Passed
      Start 4: NetExposurePositive
  4/9 Test #4: NetExposurePositive .............   Passed
      Start 5: NetExposureNegative
  5/9 Test #5: NetExposureNegative .............   Passed
      Start 6: NetExposureIgnoresLmp
  6/9 Test #6: NetExposureIgnoresLmp ...........   Passed
      Start 7: LimitBreachOver
  7/9 Test #7: LimitBreachOver .................   Passed
      Start 8: LimitBreachUnder
  8/9 Test #8: LimitBreachUnder ................   Passed
      Start 9: LimitBreachExact
  9/9 Test #9: LimitBreachExact ................   Passed

100% tests passed, 0 tests failed out of 9
```

If any test fails, check that the function logic in `lib.rs` matches the formula in `riskcalc.h` (the C++ source is still present at this point for comparison).

---

### Task 6: Build the shared library standalone (BUILD_TESTS=OFF)

This verifies the Docker build path: configure with `BUILD_TESTS=OFF` and confirm `libriskcalc.so` is produced.

**Files:** None (verification only)

- [ ] **Step 1: Clean build directory**

```bash
rm -rf /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/build
```

- [ ] **Step 2: Configure and build with BUILD_TESTS=OFF**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc
RUSTC_REAL="$(rustup which rustc)"
cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
    -DRust_COMPILER="${RUSTC_REAL}"
cmake --build build -j"$(nproc)"
```

Expected: Rust compiles, no GoogleTest download, build succeeds.

- [ ] **Step 3: Confirm .so was produced**

```bash
ls -lh /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/build/libriskcalc.so
```

Expected: file exists and is non-empty (a few hundred KB is normal for a cdylib).

- [ ] **Step 4: Verify symbols**

```bash
nm -gD /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/build/libriskcalc.so \
    | grep -E "calc_mtm_pnl|calc_net_exposure|check_limit_breach"
```

Expected: three lines, each marked `T` (text/code, exported):
```
T calc_mtm_pnl
T calc_net_exposure
T check_limit_breach
```

---

### Task 7: Delete the C++ source files

**Files:**
- Delete: `risk-svc/src/risk_calc.h`
- Delete: `risk-svc/src/risk_calc.cpp`

- [ ] **Step 1: Delete files**

```bash
rm /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/src/risk_calc.h
rm /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/src/risk_calc.cpp
```

- [ ] **Step 2: Verify build still works without them**

```bash
rm -rf /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc/build
cd /Users/kab/Projects/Mixed_RUST/trade_infra/risk-svc
RUSTC_REAL="$(rustup which rustc)"
cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
    -DRust_COMPILER="${RUSTC_REAL}"
cmake --build build -j"$(nproc)"
ls build/libriskcalc.so
```

Expected: library builds cleanly, no references to deleted files.

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git add -u risk-svc/src/risk_calc.h risk-svc/src/risk_calc.cpp
git commit -m "chore(risk-svc): delete C++ risk_calc sources replaced by Rust"
git push origin main
```

(`git add -u` stages deletions.)

---

### Task 8: Update the Dockerfile

**Files:**
- Modify: `risk-svc/Dockerfile`

Current Dockerfile for reference:
```dockerfile
# Stage 1: Build C++ shared library
FROM ubuntu:22.04 AS cpp-builder
RUN apt-get update && apt-get install -y \
    cmake build-essential git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY CMakeLists.txt ./
COPY include/ include/
COPY src/ src/
RUN cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
    && cmake --build build --target riskcalc -j"$(nproc)"

# Stage 2: Build Go binary (CGo)
FROM golang:1.26-bookworm AS go-builder
RUN apt-get update && apt-get install -y build-essential \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=cpp-builder /build/include ./include
COPY --from=cpp-builder /build/build/libriskcalc.so ./build/libriskcalc.so
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /risk-svc ./cmd/server

# Stage 3: Runtime
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=cpp-builder /build/build/libriskcalc.so /app/build/libriskcalc.so
RUN echo "/app/build" > /etc/ld.so.conf.d/riskcalc.conf && ldconfig
COPY --from=go-builder /risk-svc /usr/local/bin/risk-svc
ENV API_ADDR=:8081 \
    METRICS_ADDR=:9103
CMD ["risk-svc"]
```

- [ ] **Step 1: Write updated Dockerfile**

Replace `risk-svc/Dockerfile` with:

```dockerfile
# Stage 1: Build Rust shared library
FROM rust:latest AS rust-builder
RUN apt-get update && apt-get install -y \
    cmake build-essential git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY CMakeLists.txt ./
COPY include/ include/
COPY risk-calc-rs/ risk-calc-rs/
RUN RUSTC_REAL="$(rustup which rustc)" \
    && cmake -B build -S . -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTS=OFF \
        -DRust_COMPILER="${RUSTC_REAL}" \
    && cmake --build build -j"$(nproc)"

# Stage 2: Build Go binary (CGo)
# ${SRCDIR} for internal/riskcalc/riskcalc.go resolves to /app/internal/riskcalc/
# so ${SRCDIR}/../../include = /app/include and ${SRCDIR}/../../build = /app/build
FROM golang:1.26-bookworm AS go-builder
RUN apt-get update && apt-get install -y build-essential \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=rust-builder /build/include ./include
COPY --from=rust-builder /build/build/libriskcalc.so ./build/libriskcalc.so
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /risk-svc ./cmd/server

# Stage 3: Runtime
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=rust-builder /build/build/libriskcalc.so /app/build/libriskcalc.so
# Register /app/build so the dynamic linker finds libriskcalc.so
RUN echo "/app/build" > /etc/ld.so.conf.d/riskcalc.conf && ldconfig
COPY --from=go-builder /risk-svc /usr/local/bin/risk-svc
ENV API_ADDR=:8081 \
    METRICS_ADDR=:9103
CMD ["risk-svc"]
```

Changes made:
1. Stage 1: `ubuntu:22.04` → `rust:latest`, alias `cpp-builder` → `rust-builder`
2. Stage 1: Added `COPY risk-calc-rs/ risk-calc-rs/` (the new Rust crate)
3. Stage 1: Added `RUSTC_REAL` workaround; `cmake --build build` (no `--target riskcalc` — Corrosion builds the library as part of the default target)
4. Stage 2: Both `COPY --from=cpp-builder` → `COPY --from=rust-builder`
5. Stage 3: `COPY --from=cpp-builder` → `COPY --from=rust-builder`

- [ ] **Step 2: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git add risk-svc/Dockerfile
git commit -m "build(risk-svc): update Dockerfile to rust:latest builder for Rust cdylib"
git push origin main
```

---

### Task 9: Docker build and verify

**Files:** None (verification only)

- [ ] **Step 1: Build the risk-svc Docker image**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose build risk-svc
```

Expected: Build succeeds. You'll see `Compiling risk-calc-rs` in the output during Stage 1. The final image tag will be `infra-risk-svc` or similar.

If the build fails with "Corrosion: could not find rustc", the `RUSTC_REAL` workaround is not taking effect — verify the Dockerfile Stage 1 cmake invocation includes `-DRust_COMPILER="${RUSTC_REAL}"`.

- [ ] **Step 2: Start all services**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose up -d
```

- [ ] **Step 3: Verify risk-svc is healthy**

```bash
docker compose ps risk-svc
```

Expected: `State: Up` (or `running`). If it shows `Exited`, check logs:

```bash
docker compose logs risk-svc --tail=30
```

Common failure: library not found at runtime. If you see `error while loading shared libraries: libriskcalc.so`, confirm the Dockerfile Stage 3 still has the `ldconfig` step.

- [ ] **Step 4: Verify risk metrics flowing**

```bash
curl -s http://localhost:19090/api/v1/query?query=risk_position_mw | python3 -m json.pp
```

Expected: JSON response with `status: "success"` and data points for HB_NORTH, HB_SOUTH, HB_WEST nodes. Non-zero values confirm risk-svc is consuming position updates.

Alternatively, check Prometheus directly:

```bash
curl -s http://localhost:9103/metrics | grep -E "risk_position|limit_breach"
```

Expected: metric lines like `risk_position_mw{node="HB_NORTH"} 5` (values vary).

- [ ] **Step 5: Let it run for 30 seconds and check for limit breach metrics**

```bash
sleep 30
curl -s http://localhost:9103/metrics | grep limit_breach_total
```

Expected: counter value ≥ 0 (may be 0 if no breaches occurred; the important thing is the metric exists).

- [ ] **Step 6: Commit memory update**

Update the project memory to record v0.5.0. No code change needed — just push a tag:

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra
git tag v0.5.0
git push origin v0.5.0
```

Then save memory (handled by the orchestrating agent).
