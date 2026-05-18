# CI: tick-engine Rust Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a parallel `rust-tests` job to `.github/workflows/market-data-svc.yml` so the 6 Rust unit tests in `tick-engine` run in CI on every push/PR to `market-data-svc/**`.

**Architecture:** One new job added to the existing workflow file, parallel to `build-and-test`. Uses `dtolnay/rust-toolchain@stable` for explicit Rust setup and runs `cargo test --manifest-path market-data-svc/tick-engine/Cargo.toml` from the repo root.

**Tech Stack:** GitHub Actions, `dtolnay/rust-toolchain@stable`, `cargo test`.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `.github/workflows/market-data-svc.yml` | Modify | Add `rust-tests` job |

---

## Task 1: Add `rust-tests` job to `market-data-svc.yml`

**Files:**
- Modify: `.github/workflows/market-data-svc.yml`

- [ ] **Step 1: Read `.github/workflows/market-data-svc.yml` to confirm current content**

The file currently looks like:

```yaml
name: market-data-svc

on:
  push:
    paths: ["market-data-svc/**"]
  pull_request:
    paths: ["market-data-svc/**"]

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install libpq-dev
        run: sudo apt-get install -y libpq-dev
      - name: Configure CMake
        run: cmake -B market-data-svc/build -S market-data-svc
      - name: Build
        run: cmake --build market-data-svc/build --target market_data_svc test_marketdata -j4
      - name: Run tests
        run: cd market-data-svc/build && ctest --output-on-failure
```

- [ ] **Step 2: Write the updated file with the new `rust-tests` job appended**

The complete updated file:

```yaml
name: market-data-svc

on:
  push:
    paths: ["market-data-svc/**"]
  pull_request:
    paths: ["market-data-svc/**"]

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install libpq-dev
        run: sudo apt-get install -y libpq-dev
      - name: Configure CMake
        run: cmake -B market-data-svc/build -S market-data-svc
      - name: Build
        run: cmake --build market-data-svc/build --target market_data_svc test_marketdata -j4
      - name: Run tests
        run: cd market-data-svc/build && ctest --output-on-failure

  rust-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Run tick-engine unit tests
        run: cargo test --manifest-path market-data-svc/tick-engine/Cargo.toml
```

- [ ] **Step 3: Validate the YAML locally**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/market-data-svc.yml'))" && echo "YAML valid"
```

Expected output:
```
YAML valid
```

If Python is unavailable, use:
```bash
docker run --rm -v "$PWD":/w -w /w mikefarah/yq eval '.jobs | keys' .github/workflows/market-data-svc.yml
```

Expected: both `build-and-test` and `rust-tests` listed.

- [ ] **Step 4: Commit and push**

```bash
git add .github/workflows/market-data-svc.yml
git commit -m "ci(market-data-svc): add rust-tests job for tick-engine unit tests"
git push origin main
```

---

## Task 2: Verify the CI run on GitHub

**Files:** none — observation only.

- [ ] **Step 1: Open the Actions tab for the commit**

```bash
gh run list --limit 5
```

Expected: a run for `market-data-svc` triggered by the push. Note the run ID.

- [ ] **Step 2: Watch the run complete**

```bash
gh run watch
```

Select the most recent `market-data-svc` run. Both jobs (`build-and-test` and `rust-tests`) should appear and complete. The `rust-tests` job typically finishes in ~60s (Rust toolchain install + `cargo test` with warm cache).

- [ ] **Step 3: Confirm `rust-tests` passed**

```bash
gh run view --log | grep -A5 "rust-tests"
```

Expected output includes:
```
Run tick-engine unit tests
running 6 tests
test tests::deterministic_with_same_seed ... ok
test tests::lmp_stays_in_range ... ok
test tests::parse_lmps_extracts_prices ... ok
test tests::parse_lmps_returns_none_on_bad_json ... ok
test tests::replay_cycles_buffer ... ok
test tests::replay_uses_ou_when_empty ... ok
test result: ok. 6 passed; 0 failed
```

If `rust-tests` fails with a compilation error (e.g. linker issue on Linux), it is likely because `ureq` pulls in OpenSSL headers. Fix by adding a `sudo apt-get install -y pkg-config libssl-dev` step before the `cargo test` step:

```yaml
  rust-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Install OpenSSL dev headers
        run: sudo apt-get install -y pkg-config libssl-dev
      - name: Run tick-engine unit tests
        run: cargo test --manifest-path market-data-svc/tick-engine/Cargo.toml
```

Commit the fix if needed:
```bash
git add .github/workflows/market-data-svc.yml
git commit -m "ci(market-data-svc): install OpenSSL headers for ureq on ubuntu"
git push origin main
```

- [ ] **Step 4: Tag v0.8.0**

```bash
git tag v0.8.0
git push origin v0.8.0
```
