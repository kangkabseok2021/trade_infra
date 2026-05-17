# README v0.12.0 Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the v0.3.0 README with a current v0.12.0 document covering all 18 services, Rust cdylibs, ERCOT data feed, 3 spread-arb pairs, and CI/CD.

**Architecture:** Single file rewrite — overwrite `trade_infra/README.md` with the complete content provided in this plan. No code changes. Commit from the git root (`/Users/kab/Projects/Mixed_RUST`) and push.

**Tech Stack:** Markdown.

---

## File Map

| File | Action |
|------|--------|
| `trade_infra/README.md` | Full rewrite |

---

## Task 1: Write the new README and commit

**Files:**
- Modify: `trade_infra/README.md`

**Important:** The git root is `/Users/kab/Projects/Mixed_RUST/`. All git commands must run from there. The file is at `/Users/kab/Projects/Mixed_RUST/trade_infra/README.md`.

- [ ] **Step 1: Overwrite `trade_infra/README.md` with the following content**

Write this exact content to `/Users/kab/Projects/Mixed_RUST/trade_infra/README.md`:

---

```markdown
# trade_infra

Energy trading infrastructure: ERCOT LMP data feed (or OU simulation fallback), automated strategy execution across 3 nodes, full order/risk lifecycle, and real-time observability via Prometheus + Grafana. Built as a learning exercise in the daily workflow of an energy trading systems developer.

**v0.12.0** — 18 Docker services · 3 strategy types · 9 strategy instances · 83 tests · all CI green

---

## Architecture

```
ERCOT DAM API (or OU fallback)
    │  fetch at startup — real LMP with ERCOT_API_KEY; OU sim if absent
    ▼
market-data-svc × 3 nodes  (C++17 + Rust cdylib tick-engine)
    │  LMP tick per node → INSERT price_ticks; NOTIFY 'price_ticks'
    │
    ├── mean_reversion.py × 3   ─── rolling z-score → BUY/SELL signal
    ├── ma_crossover.py × 3     ─── EMA crossover (edge-triggered) → signal
    └── spread_arb.py × 3 pairs ─── inter-node spread z-score → paired signals
                │
                │  INSERT INTO signals; NOTIFY 'signals'
                ▼
strategy-engine  (Go)
    │  LISTEN 'signals' → risk gate (position limit + 30s cooldown)
    │  POST /orders → order-svc
    ▼
order-svc  (Go)
    │  LISTEN 'price_ticks' → evaluate PENDING orders → NOTIFY 'order_updates'
    ▼
risk-svc  (Go CGo + Rust cdylib risk-calc-rs)
    │  LISTEN 'order_updates' → MTM P&L, net exposure, limit breach
    │  INSERT risk_snapshots
    ▼
Prometheus ──scrapes all /metrics──► Grafana dashboards
```

PostgreSQL LISTEN/NOTIFY is the message bus — no Kafka, no Redis.

---

## Services

| Service | Language / Runtime | Metrics | Responsibility |
|---|---|---|---|
| market-data-svc (×3 nodes) | C++17 + Rust cdylib | :9101 | LMP tick simulation or ERCOT replay |
| order-svc | Go 1.26 | :9102 | Order lifecycle, LISTEN/NOTIFY, REST API |
| risk-svc | Go CGo + Rust cdylib | :9103 | MTM P&L, net exposure, limit breach |
| strategy-engine | Go 1.26 | :9104 | Signal gate, order submission |
| mean_reversion.py (×3) | Python 3.12 | — | OU mean-reversion signals per node |
| ma_crossover.py (×3) | Python 3.12 | — | EMA crossover signals per node |
| spread_arb.py (×3 pairs) | Python 3.12 | — | Z-score spread signals (N/S, N/W, S/W) |
| postgres | PostgreSQL 16 | — | Persistence + LISTEN/NOTIFY message bus |
| prometheus | Prometheus 2.51 | :19090 | Metrics scrape + SLO alerting |
| grafana | Grafana 10.4 | :13000 | Dashboards |

Total: **18 containers** (3 market-data + 1 order + 1 risk + 1 strategy-engine + 9 Python strategies + 1 postgres + 1 prometheus + 1 grafana).

---

## ERCOT Nodes

| Node | Base LMP | Volatility | Mean-reversion threshold |
|---|---|---|---|
| HB_NORTH | $45/MWh | σ=5 | 1.0σ |
| HB_SOUTH | $42/MWh | σ=7 | 1.0σ |
| HB_WEST | $38/MWh | σ=6 | 1.2σ (lower vol — higher bar) |

---

## ERCOT Data Feed

`tick-engine` (Rust cdylib) fetches ERCOT DAM Settlement Point Prices at container startup via the `Ocp-Apim-Subscription-Key` header. Without a key, ticks fall back to an Ornstein-Uhlenbeck simulation using each node's `BASE_LMP` and `VOLATILITY`.

**To enable real ERCOT data:**

1. Register at [developer.ercot.com](https://developer.ercot.com) to get an API key
2. Set it in `infra/docker-compose.yml` for each `market-data-svc`:
   ```yaml
   ERCOT_API_KEY: "your-key-here"
   ```
3. Optionally set the replay date (default `2024-01-15`):
   ```yaml
   ERCOT_REPLAY_DATE: "YYYY-MM-DD"
   ```

On any fetch failure (network, invalid key, API change), the service logs:
```
tick-engine: ERCOT fetch failed for HB_NORTH, using OU fallback
```
and falls back silently — ticks always flow.

---

## Strategies

| Strategy | Instances | Signal logic | Config params |
|---|---|---|---|
| mean_reversion | ×3 nodes | BUY when LMP < mean − threshold·σ; SELL above | window, threshold, quantity_mw |
| ma_crossover | ×3 nodes | BUY/SELL on fast/slow EMA crossover (fires once per cross) | fast_period, slow_period, quantity_mw |
| spread_arb | ×3 pairs | BUY nodeA/SELL nodeB when z-score(nodeA−nodeB) < −threshold; reverse above | window, threshold, quantity_mw |

Spread-arb pairs: **HB_NORTH/HB_SOUTH**, **HB_NORTH/HB_WEST**, **HB_SOUTH/HB_WEST**.

All configs live in `sql/schema.sql` (`strategy_configs` table) and are tunable at runtime — see [Strategy Tuning](#strategy-tuning).

---

## Database

| Table | Purpose |
|---|---|
| `price_ticks` | LMP tick history per node |
| `orders` | Bid lifecycle (PENDING → FILLED / CANCELLED) |
| `positions` | Net MW and average fill price per node |
| `risk_snapshots` | MTM P&L, exposure, limit headroom per fill |
| `signals` | Full audit trail: strategy → signal → order linkage |
| `strategy_configs` | Tunable params per strategy per node |

---

## Docker Compose

Starts all 18 services in dependency order:

```bash
cd infra
docker compose up --build
```

| Endpoint | URL |
|---|---|
| order-svc REST | http://localhost:18080 |
| risk-svc REST | http://localhost:8081 |
| strategy-engine metrics | http://localhost:9104/metrics |
| Prometheus | http://localhost:19090 |
| Grafana | http://localhost:13000 (admin / admin) |
| PostgreSQL | localhost:5433 |

> Ports 18080 / 19090 / 13000 / 5433 are remapped from defaults to avoid conflicts with common local services.

Submit an order manually:

```bash
curl -s -X POST http://localhost:18080/orders \
  -H 'Content-Type: application/json' \
  -d '{"node":"HB_NORTH","side":"BUY","quantity_mw":10,"limit_price":60}'
```

Tear down:

```bash
docker compose down
```

> `docker compose down` removes the postgres container and all data. The schema is re-applied from `sql/schema.sql` on the next `up`.

---

## Running Tests

### Rust (cargo test)

```bash
# tick-engine — 6 tests (OU generator, ERCOT JSON parser, replay buffer)
source "$HOME/.cargo/env"
cd market-data-svc/tick-engine
cargo test

# risk-calc-rs — 9 tests (MTM P&L, net exposure, limit breach)
cd risk-svc/risk-calc-rs
cargo test
```

### C++ (GoogleTest via CMake)

```bash
# market-data-svc — 5 tests
cmake -B market-data-svc/build -S market-data-svc \
  -DRust_COMPILER="$(rustup which rustc)"
cmake --build market-data-svc/build --target test_marketdata -j4
cd market-data-svc/build && ctest --output-on-failure

# risk-svc — 9 tests
cmake -B risk-svc/build -S risk-svc \
  -DRust_COMPILER="$(rustup which rustc)"
cmake --build risk-svc/build --target test_riskcalc -j4
cd risk-svc/build && ctest --output-on-failure
```

### Go

```bash
# order-svc — 8 tests (requires running postgres)
cd order-svc
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v

# risk-svc — 6 tests (requires running postgres)
cd risk-svc
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v

# strategy-engine — 13 tests
cd strategy-engine
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v
```

### Python

```bash
cd python
uv sync --extra dev
uv run ruff check .
uv run pytest tests/ -v   # 27 tests
```

**Total: 83 tests** — 6 Rust tick-engine · 9 Rust risk-calc-rs · 5 C++ market-data · 9 C++ risk-calc · 8 Go order · 6 Go risk · 13 Go strategy-engine · 27 Python

---

## CI/CD

Five GitHub Actions workflows. All workflows carry `working-directory: trade_infra` and path filters prefixed with `trade_infra/`.

| Workflow | Trigger | Jobs |
|---|---|---|
| `market-data-svc` | `trade_infra/market-data-svc/**` | CMake + ctest (C++ GoogleTest); `cargo test` (Rust tick-engine) |
| `order-svc` | `trade_infra/order-svc/**` | Go tests with postgres service container |
| `python` | `trade_infra/python/**` | ruff lint; pytest (27 tests) |
| `risk-svc` | `trade_infra/risk-svc/**` | CMake + ctest; Go CGo tests; `cargo test` (Rust risk-calc-rs) |
| `integration` | push to `main` | Full Docker Compose stack + smoke test |

> Workflow files live at `.github/workflows/` relative to the git root (`Mixed_RUST/`), not inside `trade_infra/`.

---

## Monitoring

Prometheus scrapes all service `/metrics` endpoints every 15 seconds. Strategy metrics carry `{strategy, node}` labels.

| Metric | Service | Description |
|---|---|---|
| `market_data_tick_total` | market-data-svc ×3 | Price ticks per node |
| `order_svc_orders_filled_total` | order-svc | Orders filled |
| `order_svc_eval_latency_seconds` | order-svc | Evaluation latency histogram |
| `risk_svc_snapshots_saved_total` | risk-svc | Risk snapshots written |
| `risk_svc_limit_breaches_total` | risk-svc | Position limit breaches |
| `strategy_engine_signals_received_total` | strategy-engine | Signals per strategy+node |
| `strategy_engine_signals_submitted_total` | strategy-engine | Orders placed per strategy+node |
| `strategy_engine_signals_skipped_total` | strategy-engine | Gate rejections by reason+node |

### SLOs

| SLO | Target | Alert |
|---|---|---|
| Order evaluation latency p99 | < 50ms | `OrderEvalLatencyHigh` (warning, 2m) |
| All services up | 100% | `ServiceDown` (critical, 1m) |

---

## Strategy Tuning

| Parameter | Location | Effect |
|---|---|---|
| `COOLDOWN_SECS` | strategy-engine env | Min seconds between orders per strategy+node |
| `POSITION_LIMIT_MW` | strategy-engine env | Gate blocks new orders when exposure ≥ limit |
| `RISK_POSITION_LIMIT_MW` | risk-svc env | Hard limit for `limit_headroom` calculation |
| `threshold` | strategy_configs table | σ multiplier for mean-reversion / spread-arb band |
| `quantity_mw` | strategy_configs table | Order size per signal per node |

Live tune example (no restart needed for params, restart strategy container to pick up new values):

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "UPDATE strategy_configs SET param_value='0.8'
   WHERE strategy='mean_reversion' AND param_key='threshold' AND node='HB_NORTH';"
docker compose restart mean-reversion
```

---

## Repo Layout

```
trade_infra/
├── market-data-svc/
│   ├── tick-engine/          Rust cdylib — ERCOT replay + OU tick generator
│   ├── src/                  C++17 DB writer (libpq)
│   └── include/marketdata.h  extern "C" ABI
├── order-svc/                Go: bid lifecycle, LISTEN/NOTIFY, REST API
├── risk-svc/
│   ├── risk-calc-rs/         Rust cdylib — MTM P&L, net exposure, limit breach
│   ├── cmd/server/           Go CGo server
│   └── include/riskcalc.h    extern "C" ABI
├── strategy-engine/          Go: signal gate, order submission, metrics
├── python/
│   ├── strategies/
│   │   ├── base.py           LISTEN/NOTIFY helpers, emit_signal
│   │   ├── mean_reversion.py
│   │   ├── ma_crossover.py
│   │   └── spread_arb.py
│   ├── data_gen.py           synthetic LMP seeder (local dev)
│   ├── analytics.py          P&L + position CLI
│   └── smoke_test.py         integration smoke test
├── sql/schema.sql            PostgreSQL schema + strategy_configs seed
├── infra/
│   ├── docker-compose.yml    18-service stack
│   ├── prometheus.yml
│   └── grafana/
└── docs/superpowers/         specs and plans
```

> Workflow files live at `(repo root)/.github/workflows/` (outside `trade_infra/`).

---

## Local Dev (without Docker)

### Prerequisites

| Tool | Version | Install |
|---|---|---|
| Rust | stable | [rustup.rs](https://rustup.rs) |
| CMake | ≥ 3.20 | `brew install cmake` |
| C++ compiler | C++17 | Xcode CLT / `apt-get install build-essential` |
| libpq | any | `brew install postgresql` / `apt-get install libpq-dev` |
| Go | ≥ 1.26 | [go.dev/dl](https://go.dev/dl) |
| Python | ≥ 3.12 | `brew install python@3.12` |
| uv | latest | `pip install uv` |
| PostgreSQL | ≥ 16 | `brew install postgresql@16` |
| Docker + Compose | v2 | [docs.docker.com](https://docs.docker.com) |

### 1. Database

```bash
createdb trade_infra
createdb trade_infra_test
psql trade_infra -f sql/schema.sql
psql trade_infra_test -f sql/schema.sql
```

### 2. market-data-svc

```bash
cmake -B market-data-svc/build -S market-data-svc \
  -DRust_COMPILER="$(rustup which rustc)"
cmake --build market-data-svc/build -j4

# Run one instance per node
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra" \
NODE_NAME=HB_NORTH BASE_LMP=45.0 VOLATILITY=5.0 \
./market-data-svc/build/market_data_svc &
```

### 3. order-svc

```bash
cd order-svc
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
go run ./cmd/server
```

### 4. risk-svc

```bash
cmake -B risk-svc/build -S risk-svc \
  -DRust_COMPILER="$(rustup which rustc)"
cmake --build risk-svc/build -j4

cd risk-svc
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
go run ./cmd/server
```

### 5. strategy-engine + strategies

```bash
cd strategy-engine
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
ORDER_SVC_URL=http://localhost:8080 \
go run ./cmd/server &

cd python
uv sync
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
uv run python strategies/mean_reversion.py --node HB_NORTH &
```

### 6. Analytics

```bash
cd python
uv run python analytics.py \
  --db-url "postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable"
```
```

---

- [ ] **Step 2: Verify the file was written and looks correct**

```bash
wc -l /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
head -5 /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
grep "v0.12.0" /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
grep "18 containers\|18 Docker" /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
grep "83 tests\|83 test" /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
grep "ERCOT_API_KEY" /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
grep "spread_arb\|spread-arb" /Users/kab/Projects/Mixed_RUST/trade_infra/README.md | wc -l
grep "CI/CD" /Users/kab/Projects/Mixed_RUST/trade_infra/README.md
```

Expected:
- `head -5` shows `# trade_infra` and tagline
- `grep v0.12.0` finds the version line
- `grep "18 containers"` finds the container count
- `grep "83 tests"` finds the test total
- `grep ERCOT_API_KEY` finds the data feed section
- `grep spread_arb` count ≥ 5 (multiple mentions)
- `grep CI/CD` finds the CI section

- [ ] **Step 3: Commit and push from git root**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/README.md
git commit -m "docs(readme): rewrite for v0.12.0 — Rust cdylibs, spread-arb, ERCOT feed, CI/CD"
git push origin main
```

Expected: successful push to `github.com:kangkabseok2021/trade_infra.git`.
