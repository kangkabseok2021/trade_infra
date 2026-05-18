# CI: risk-calc-rs Rust Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a parallel `rust-tests` job to `.github/workflows/risk-svc.yml` so the 9 Rust unit tests in `risk-svc/risk-calc-rs/` run in CI on every push/PR to `trade_infra/risk-svc/**`.

**Architecture:** One new job appended to the existing workflow, parallel to `cpp-tests` and `go-tests` (no `needs:`). Uses `dtolnay/rust-toolchain@stable` and runs `cargo test` from the `trade_infra` working directory. `risk-calc-rs` has no dependencies — no apt installs required.

**Tech Stack:** GitHub Actions, `dtolnay/rust-toolchain@stable`, `cargo test`.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `.github/workflows/risk-svc.yml` | Modify | Add `rust-tests` job |

**Important:** The workflow file lives at the **repository root**, not inside `trade_infra/`. The absolute path is `/Users/kab/Projects/Mixed_RUST/.github/workflows/risk-svc.yml`. The git root is `/Users/kab/Projects/Mixed_RUST/`.

---

## Task 1: Add `rust-tests` job to `risk-svc.yml`

**Files:**
- Modify: `.github/workflows/risk-svc.yml` (at `/Users/kab/Projects/Mixed_RUST/.github/workflows/risk-svc.yml`)

- [ ] **Step 1: Read the current file to confirm its content**

```bash
cat /Users/kab/Projects/Mixed_RUST/.github/workflows/risk-svc.yml
```

Expected: file has two jobs — `cpp-tests` and `go-tests`. No `rust-tests` job yet.

- [ ] **Step 2: Append the `rust-tests` job**

The complete updated file:

```yaml
name: risk-svc

on:
  push:
    paths: ["trade_infra/risk-svc/**"]
  pull_request:
    paths: ["trade_infra/risk-svc/**"]

jobs:
  cpp-tests:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: trade_infra
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - name: Configure and build
        run: |
          cmake -B risk-svc/build -S risk-svc -DRust_COMPILER="$(rustup which rustc)"
          cmake --build risk-svc/build --target test_riskcalc -j4
      - name: Run C++ tests
        run: cd risk-svc/build && ctest --output-on-failure

  go-tests:
    runs-on: ubuntu-latest
    needs: cpp-tests
    defaults:
      run:
        working-directory: trade_infra
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: trade_infra_test
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-retries 10
        ports: ["5432:5432"]
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Build libriskcalc
        run: |
          cmake -B risk-svc/build -S risk-svc -DRust_COMPILER="$(rustup which rustc)"
          cmake --build risk-svc/build --target riskcalc -j4

      - name: Apply schema
        run: psql postgresql://postgres:postgres@localhost:5432/trade_infra_test -f sql/schema.sql

      - name: Run Go tests
        working-directory: trade_infra/risk-svc
        env:
          TEST_DATABASE_URL: postgresql://postgres:postgres@localhost:5432/trade_infra_test?sslmode=disable
        run: go test ./internal/risk/... -v

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

- [ ] **Step 3: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('/Users/kab/Projects/Mixed_RUST/.github/workflows/risk-svc.yml'))" && echo "YAML valid"
```

Expected: `YAML valid`

- [ ] **Step 4: Commit and push from the repo root**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add .github/workflows/risk-svc.yml
git commit -m "ci(risk-svc): add rust-tests job for risk-calc-rs unit tests"
git push origin main
```

---

## Task 2: Verify CI run and tag v0.9.0

**Files:** none — observation only.

The `paths: ["trade_infra/risk-svc/**"]` filter means the workflow only triggers when a file under `trade_infra/risk-svc/**` is changed. The commit in Task 1 only changed `.github/workflows/risk-svc.yml` — which is NOT under that path. A second trigger commit is needed.

- [ ] **Step 1: Touch a file in `trade_infra/risk-svc/` to trigger the workflow**

```bash
cd /Users/kab/Projects/Mixed_RUST
echo "" >> trade_infra/risk-svc/risk-calc-rs/Cargo.toml
git add trade_infra/risk-svc/risk-calc-rs/Cargo.toml
git commit -m "ci: trigger risk-svc workflow for rust-tests job verification"
git push origin main
```

- [ ] **Step 2: Watch the run**

```bash
gh run list --repo kangkabseok2021/trade_infra --limit 5
```

Wait ~2 minutes for the run to complete. The `rust-tests` job has no CMake build — it's the fastest of the three jobs (~45s total: toolchain install + `cargo test`).

```bash
gh run watch --repo kangkabseok2021/trade_infra
```

- [ ] **Step 3: Confirm all three jobs passed**

```bash
gh api repos/kangkabseok2021/trade_infra/actions/runs \
  --jq '.workflow_runs[] | select(.name=="risk-svc") | {id, conclusion, created_at}' \
  | head -20
```

Then check the jobs for the latest run ID:

```bash
gh api repos/kangkabseok2021/trade_infra/actions/runs/<RUN_ID>/jobs \
  --jq '.jobs[] | {name, conclusion}'
```

Expected:
```json
{"name":"cpp-tests","conclusion":"success"}
{"name":"go-tests","conclusion":"success"}
{"name":"rust-tests","conclusion":"success"}
```

If `rust-tests` fails with a compile error, check the logs:
```bash
gh run view <RUN_ID> --log | grep -A 20 "rust-tests"
```

`risk-calc-rs` has no dependencies so the only likely failure is a missing `Cargo.toml` path — verify with `--manifest-path risk-svc/risk-calc-rs/Cargo.toml` from `trade_infra/` working directory.

- [ ] **Step 4: Clean up the trigger commit's trailing newline**

```bash
cd /Users/kab/Projects/Mixed_RUST
# Remove the trailing blank line added to Cargo.toml
# Read the file, strip trailing newline, rewrite
python3 -c "
content = open('trade_infra/risk-svc/risk-calc-rs/Cargo.toml').read().rstrip('\n')
open('trade_infra/risk-svc/risk-calc-rs/Cargo.toml', 'w').write(content + '\n')
"
git add trade_infra/risk-svc/risk-calc-rs/Cargo.toml
git commit -m "chore: restore Cargo.toml after CI trigger"
git push origin main
```

- [ ] **Step 5: Tag v0.9.0**

```bash
cd /Users/kab/Projects/Mixed_RUST
git tag v0.9.0
git push origin v0.9.0
```
