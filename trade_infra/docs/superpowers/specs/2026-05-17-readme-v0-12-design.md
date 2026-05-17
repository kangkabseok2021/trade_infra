# README v0.12.0 Rewrite Design

**Date:** 2026-05-17
**Version:** v0.13.0 (README is the only deliverable)

## Goal

Replace the v0.3.0 README with a current v0.12.0 document covering all services, Rust cdylibs, 3 spread-arb pairs, ERCOT data feed, and CI/CD.

## Section Specifications

### Header

- Title: `# trade_infra`
- Tagline: one-line description of the system (energy trading infra, ERCOT LMP simulation → real data, automated strategy execution, full observability)
- Version: **v0.12.0** — 20 Docker services, 3 strategy types × 9 instances, all CI green

---

### Architecture

Updated ASCII diagram. Key changes from v0.3.0:
- `market-data-svc` is now **Rust cdylib** (`tick-engine/`) behind the `extern "C"` ABI — replaces C++17 OU generator
- `risk-svc` is now **Rust cdylib** (`risk-calc-rs/`) behind CGo — replaces C++ `libriskcalc.so`
- Add `spread_arb.py × 3 pairs` alongside mean_reversion and ma_crossover
- Note ERCOT data source (with OU fallback)

```
ERCOT DAM API (or OU fallback)
    │  fetch at startup (ERCOT_API_KEY → real LMP; empty → OU sim)
    ▼
market-data-svc × 3 nodes  (C++ + Rust cdylib tick-engine)
    │  LMP tick per node → price_ticks; NOTIFY 'price_ticks'
    ▼
mean_reversion.py × 3          ma_crossover.py × 3       spread_arb.py × 3 pairs
    │  OU mean-reversion             EMA crossover             z-score on spread
    │                               INSERT INTO signals, NOTIFY 'signals'
    ▼
strategy-engine  (Go)
    │  risk gate: position limit + 30s cooldown → POST /orders
    ▼
order-svc  (Go)
    │  LISTEN 'price_ticks' → evaluate PENDING → NOTIFY 'order_updates'
    ▼
risk-svc  (Go + Rust cdylib risk-calc-rs)
    │  CGo: MTM P&L, net exposure, limit breach → risk_snapshots
    ▼
Prometheus ──► Grafana
```

---

### Services Table

20 containers total. Table columns: Service | Language/Runtime | Metrics | Responsibility

| Service | Language | Metrics | Responsibility |
|---|---|---|---|
| market-data-svc (×3) | C++ + Rust cdylib | :9101 | LMP ticks (ERCOT replay or OU sim) |
| order-svc | Go 1.26 | :9102 | Order lifecycle, REST API |
| risk-svc | Go CGo + Rust cdylib | :9103 | MTM P&L, net exposure, limits |
| strategy-engine | Go 1.26 | :9104 | Signal gate, order submission |
| mean_reversion.py (×3) | Python 3.12 | — | Mean-reversion signals per node |
| ma_crossover.py (×3) | Python 3.12 | — | EMA crossover signals per node |
| spread_arb.py (×3 pairs) | Python 3.12 | — | Z-score spread signals per pair |
| postgres | PostgreSQL 16 | — | Message bus + persistence |
| prometheus | Prometheus 2.51 | :19090 | Metrics scrape + SLO alerting |
| grafana | Grafana 10.4 | :13000 | Dashboards |

---

### ERCOT Nodes

Same 3 nodes, same params as before. Keep existing table.

---

### ERCOT Data Feed (NEW section)

- `tick-engine` fetches ERCOT DAM Settlement Point Prices at startup via `Ocp-Apim-Subscription-Key` header
- Without a key → OU fallback (ticks still flow)
- How to get a key: `developer.ercot.com`
- How to set it: `ERCOT_API_KEY: "your-key"` in `infra/docker-compose.yml` for each market-data-svc service
- Replay date: `ERCOT_REPLAY_DATE: "YYYY-MM-DD"` — default `2024-01-15`

---

### Strategies

Three strategy types, 9 total instances:

| Strategy | Instances | Logic | Params |
|---|---|---|---|
| mean_reversion | ×3 nodes | BUY/SELL when LMP crosses ±threshold·σ of rolling mean | window, threshold, quantity_mw |
| ma_crossover | ×3 nodes | BUY/SELL on fast/slow EMA cross (edge-triggered) | fast_period, slow_period, quantity_mw |
| spread_arb | ×3 pairs (NS, NW, SW) | BUY/SELL when z-score of (nodeA − nodeB) spread crosses ±threshold | window, threshold, quantity_mw |

Spread-arb pairs: HB_NORTH/HB_SOUTH, HB_NORTH/HB_WEST, HB_SOUTH/HB_WEST.

---

### Database

Same table list as v0.3.0 — unchanged.

---

### Docker Compose (Quick Start)

Primary path. Commands:
```bash
cd infra
docker compose up --build
```

Port table (same as v0.3.0 — ports haven't changed).

ERCOT key note: set `ERCOT_API_KEY` for real data.

---

### Running Tests

Updated counts and commands. Key changes:
- Add `cargo test` for `tick-engine` (6 Rust tests)
- Add `cargo test` for `risk-calc-rs` (9 Rust tests)
- Python tests: now 27 (was 22)
- Total: 83 tests (was 49): 5 C++ market-data + 9 C++ risk-calc + 6 Rust tick-engine + 9 Rust risk-calc-rs + 8 Go order + 6 Go risk + 13 Go strategy-engine + 27 Python

Commands for each suite with expected output.

---

### CI/CD (NEW section)

Table of 5 GitHub Actions workflows:

| Workflow | Trigger path | Jobs |
|---|---|---|
| market-data-svc | `trade_infra/market-data-svc/**` | CMake+ctest (C++ GoogleTest) + cargo test (Rust) |
| order-svc | `trade_infra/order-svc/**` | Go tests (with postgres service) |
| python | `trade_infra/python/**` | ruff lint + pytest |
| risk-svc | `trade_infra/risk-svc/**` | CMake+ctest + Go CGo tests + cargo test |
| integration | push to main | Full Docker Compose stack + smoke test |

---

### Monitoring

Same metrics table + SLOs as v0.3.0.

---

### Strategy Tuning

Same params table as v0.3.0.

---

### Repo Layout

Updated tree:
- `market-data-svc/tick-engine/` — Rust cdylib (was C++)
- `risk-svc/risk-calc-rs/` — Rust cdylib (was C++)
- `python/strategies/` — add `spread_arb.py`
- `.github/workflows/` — 5 CI workflows (note: at repo root, not inside trade_infra/)

---

### Local Dev (without Docker)

Keep the step-by-step guide from v0.3.0 with these updates:
- Add Rust to prerequisites table
- CMake commands now need `-DRust_COMPILER="$(rustup which rustc)"` for Corrosion
- Build commands: `cmake --build build` (no `--target riskcalc/marketdata` — Corrosion targets aren't directly buildable)

## Scope

- One file: `README.md` (full rewrite)
- No code changes
- Commit + push
