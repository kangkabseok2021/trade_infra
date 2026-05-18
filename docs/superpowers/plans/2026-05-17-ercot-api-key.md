# ERCOT API Key Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ERCOT_API_KEY` env var support so `fetch_ercot_lmps` sends the `Ocp-Apim-Subscription-Key` header required by the ERCOT WAF.

**Architecture:** One new line reads `ERCOT_API_KEY` from env (empty string when unset) and one `.set()` call adds the header to the ureq request chain. Empty key → 403 → `.call().ok()?` → `None` → OU fallback — identical to current behaviour. `ERCOT_API_KEY: ""` added to docker-compose for discoverability.

**Tech Stack:** Rust 2021, `ureq 2`, Docker Compose.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `market-data-svc/tick-engine/src/lib.rs` | Modify | Add `api_key` read + `.set()` in `fetch_ercot_lmps` |
| `infra/docker-compose.yml` | Modify | Add `ERCOT_API_KEY: ""` to 3 market-data-svc services |

---

## Task 1: Add API key header to `fetch_ercot_lmps` and `ERCOT_API_KEY` to docker-compose

**Files:**
- Modify: `market-data-svc/tick-engine/src/lib.rs` (lines 65–67)
- Modify: `infra/docker-compose.yml` (lines 27, 44, 60 — after each `ERCOT_REPLAY_DATE`)

**Important:** The git root is `/Users/kab/Projects/Mixed_RUST/`. All git commands must run from there. The Rust crate is at `market-data-svc/tick-engine/` inside `trade_infra/`. `cargo` is at `~/.cargo/bin/cargo` — source `$HOME/.cargo/env` before running it.

- [ ] **Step 1: Edit `market-data-svc/tick-engine/src/lib.rs`**

Find this block (lines 65–70):
```rust
    let body = ureq::get(&url)
        .timeout(std::time::Duration::from_secs(5))
        .call()
        .ok()?
        .into_string()
        .ok()?;
```

Replace it with:
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

- [ ] **Step 2: Run all 6 Rust unit tests — all must pass**

```bash
source "$HOME/.cargo/env"
cd /Users/kab/Projects/Mixed_RUST/trade_infra/market-data-svc/tick-engine
cargo test 2>&1 | tail -10
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

The `.set()` call does not affect any unit test because `fetch_ercot_lmps` is not called by any test (only `parse_ercot_json` and the replay/OU logic are tested directly).

- [ ] **Step 3: Edit `infra/docker-compose.yml` — add `ERCOT_API_KEY` to all 3 market-data-svc services**

For `market-data-svc` (HB_NORTH), add after line 27 (`ERCOT_REPLAY_DATE: "2024-01-15"`):
```yaml
      ERCOT_API_KEY: ""
```

For `market-data-svc-south` (HB_SOUTH), add after its `ERCOT_REPLAY_DATE: "2024-01-15"`:
```yaml
      ERCOT_API_KEY: ""
```

For `market-data-svc-west` (HB_WEST), add after its `ERCOT_REPLAY_DATE: "2024-01-15"`:
```yaml
      ERCOT_API_KEY: ""
```

Result for each service environment block:
```yaml
      NODE_NAME: HB_NORTH        # (or SOUTH/WEST)
      BASE_LMP: "45.0"           # (varies per service)
      VOLATILITY: "5.0"          # (varies per service)
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
      ERCOT_REPLAY_DATE: "2024-01-15"
      ERCOT_API_KEY: ""
```

- [ ] **Step 4: Validate docker-compose YAML**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose config --quiet
```

Expected: no output (silent success).

- [ ] **Step 5: Commit both files and push from the repo root**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/market-data-svc/tick-engine/src/lib.rs
git add trade_infra/infra/docker-compose.yml
git commit -m "feat(tick-engine): add ERCOT_API_KEY support — Ocp-Apim-Subscription-Key header"
git push origin main
```

---

## Task 2: Verify CI and tag v0.11.0

**Files:** none — observation only.

The `market-data-svc` workflow triggers on `paths: ["trade_infra/market-data-svc/**"]`. The commit in Task 1 touches `trade_infra/market-data-svc/tick-engine/src/lib.rs` — this matches. The workflow will run both `build-and-test` and `rust-tests` jobs.

- [ ] **Step 1: Check that the CI run was triggered**

```bash
gh api repos/kangkabseok2021/trade_infra/actions/runs \
  --jq '.workflow_runs[:5][] | {name, status, conclusion, sha: .head_sha[:7]}' 2>&1
```

Expected: a `market-data-svc` run in `queued` or `in_progress` state for the latest commit SHA.

- [ ] **Step 2: Wait for it to complete**

```bash
gh run watch --repo kangkabseok2021/trade_infra
```

Select the most recent `market-data-svc` run. Both `build-and-test` and `rust-tests` should pass (~3–5 min).

- [ ] **Step 3: Confirm both jobs passed**

```bash
gh api repos/kangkabseok2021/trade_infra/actions/runs \
  --jq '[.workflow_runs[] | select(.name=="market-data-svc")][0] | .id' 2>&1
```

Then:
```bash
gh api repos/kangkabseok2021/trade_infra/actions/runs/<RUN_ID>/jobs \
  --jq '.jobs[] | {name, conclusion}' 2>&1
```

Expected:
```json
{"name":"build-and-test","conclusion":"success"}
{"name":"rust-tests","conclusion":"success"}
```

- [ ] **Step 4: Tag v0.11.0**

```bash
cd /Users/kab/Projects/Mixed_RUST
git tag v0.11.0
git push origin v0.11.0
```
