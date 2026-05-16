# trade_infra

Energy trading infrastructure covering the full daily workflow of a trading system developer: multi-node LMP simulation, automated strategy execution, order management, and risk/P&L monitoring — wired through PostgreSQL and observable via Prometheus + Grafana.

**v0.3.0** — 3 ERCOT nodes (HB_NORTH, HB_SOUTH, HB_WEST), 2 automated strategies, 15 Docker services, 49 tests.

---

## Architecture

```
market-data-svc × 3 nodes  (C++17)
    │  Ornstein-Uhlenbeck LMP per node → price_ticks
    │  NOTIFY 'price_ticks'
    ▼
mean_reversion.py × 3      (Python)     ma_crossover.py × 3  (Python)
    │  rolling window → BUY/SELL signal     EMA crossover → BUY/SELL signal
    │  INSERT INTO signals, NOTIFY 'signals'
    ▼
strategy-engine            (Go)
    │  LISTEN 'signals' → risk gate (position limit + cooldown)
    │  POST /orders → order-svc
    ▼
order-svc                  (Go)
    │  LISTEN 'price_ticks' → evaluate PENDING orders
    │  NOTIFY 'order_updates'
    ▼
risk-svc                   (C++ libriskcalc.so + Go CGo)
    │  LISTEN 'order_updates' → MTM P&L, net exposure per node
    │  writes risk_snapshots
    ▼
Prometheus  ──scrapes /metrics──►  all services (strategy+node labels)
    ▼
Grafana  ──dashboards──►  tick rate, fills, signals, availability
```

### Services

| Service | Language | Metrics port | Responsibility |
|---|---|---|---|
| market-data-svc (×3 nodes) | C++17 + libpq | 9101 | LMP tick simulation per node |
| order-svc | Go 1.26 | 9102 | Bid lifecycle, LISTEN/NOTIFY, REST API |
| risk-svc | C++ CGo + Go | 9103 | MTM P&L, net exposure, limit breach |
| strategy-engine | Go 1.26 | 9104 | Signal consumption, risk gate, order submission |
| mean_reversion.py (×3) | Python | — | Mean-reversion signals per node |
| ma_crossover.py (×3) | Python | — | EMA crossover signals per node |

### ERCOT Nodes

| Node | Base LMP | Volatility | Strategy threshold |
|---|---|---|---|
| HB_NORTH | $45/MWh | σ=5 | 1.0σ |
| HB_SOUTH | $42/MWh | σ=7 | 1.0σ |
| HB_WEST | $38/MWh | σ=6 | 1.2σ (lower vol — higher bar) |

### Database tables

| Table | Purpose |
|---|---|
| `price_ticks` | LMP tick history per node |
| `orders` | Bid lifecycle (PENDING → FILLED / REJECTED) |
| `positions` | Net MW and average fill price per node |
| `risk_snapshots` | MTM P&L, exposure, limit headroom per fill |
| `signals` | Full audit trail: strategy → signal → order linkage |
| `strategy_configs` | Tunable params per strategy per node |

---

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| CMake | ≥ 3.20 | `brew install cmake` |
| C++ compiler | C++17 | Xcode CLT / `apt-get install build-essential` |
| libpq | any | `brew install postgresql` / `apt-get install libpq-dev` |
| Go | ≥ 1.26 | [go.dev/dl](https://go.dev/dl) |
| Python | ≥ 3.12 | `brew install python@3.12` |
| uv | latest | `pip install uv` |
| PostgreSQL | ≥ 16 | `brew install postgresql@16` |
| Docker + Compose | v2 | [docs.docker.com](https://docs.docker.com) |

---

## Quick Start (local)

### 1. Database

```bash
createdb trade_infra
createdb trade_infra_test
psql trade_infra -f sql/schema.sql
psql trade_infra_test -f sql/schema.sql
```

### 2. Seed synthetic price history

```bash
cd python
uv sync
uv run python data_gen.py --nodes HB_NORTH,HB_SOUTH,HB_WEST --ticks 3600
```

### 3. market-data-svc (per node)

```bash
cd market-data-svc
cmake -B build -S . && cmake --build build --target market_data_svc -j4

# Run one instance per node
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra" \
NODE_NAME=HB_NORTH BASE_LMP=45.0 VOLATILITY=5.0 ./build/market_data_svc &
```

### 4. order-svc

```bash
cd order-svc
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
go run ./cmd/server
```

Submit an order manually:

```bash
curl -s -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"node":"HB_NORTH","side":"BUY","quantity_mw":10,"limit_price":60}'
```

### 5. risk-svc

```bash
cd risk-svc
cmake -B build -S . && cmake --build build --target riskcalc -j4
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
go run ./cmd/server
```

### 6. strategy-engine

```bash
cd strategy-engine
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
ORDER_SVC_URL=http://localhost:8080 \
go run ./cmd/server &

# Run strategies for each node
cd python
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
uv run python strategies/mean_reversion.py --node HB_NORTH &
```

### 7. Analytics

```bash
cd python
uv run python analytics.py --db-url "postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable"
```

---

## Docker Compose

Starts all 15 services (3 nodes × market-data + 3 nodes × 2 strategies + strategy-engine + order-svc + risk-svc + postgres + prometheus + grafana):

```bash
cd infra
docker compose up --build
```

| Service | URL |
|---|---|
| order-svc REST | http://localhost:18080 |
| risk-svc REST | http://localhost:8081 |
| strategy-engine metrics | http://localhost:9104/metrics |
| Prometheus | http://localhost:19090 |
| Grafana | http://localhost:13000 (admin / admin) |
| PostgreSQL | localhost:5433 |

> Ports 18080 / 19090 / 13000 / 5433 are remapped to avoid conflicts with common local services.

Tear down (removes volumes):

```bash
docker compose down -v
```

---

## Running Tests

### C++ (GoogleTest via CMake)

```bash
# market-data-svc — 5 tests
cmake -B market-data-svc/build -S market-data-svc
cmake --build market-data-svc/build --target test_marketdata -j4
cd market-data-svc/build && ctest --output-on-failure

# risk-svc — 9 tests
cmake -B risk-svc/build -S risk-svc
cmake --build risk-svc/build --target test_riskcalc -j4
cd risk-svc/build && ctest --output-on-failure
```

### Go

```bash
# order-svc — 8 tests
cd order-svc
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v

# risk-svc — 6 tests
cd risk-svc
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v

# strategy-engine — 13 tests
cd strategy-engine
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v
```

### Python (pytest + ruff)

```bash
cd python
uv sync --extra dev
uv run ruff check .       # lint (22 tests)
uv run pytest tests/ -v   # 22 tests
```

**Total: 49 tests** (5 C++ market-data + 9 C++ risk-calc + 8 Go order + 6 Go risk + 13 Go strategy-engine + 22 Python)

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

### Strategy tuning

| Parameter | Location | Effect |
|---|---|---|
| `COOLDOWN_SECS` | strategy-engine env | Min seconds between orders per strategy+node |
| `POSITION_LIMIT_MW` | strategy-engine env | Gate blocks new orders when exposure ≥ limit |
| `RISK_POSITION_LIMIT_MW` | risk-svc env | Hard limit for `limit_headroom` calculation |
| `threshold` | strategy_configs table | σ multiplier for mean-reversion band width |
| `quantity_mw` | strategy_configs table | Order size per signal per node |

---

## Rust Migration Path

Every C++ computation is isolated in a shared library with a clean `extern "C"` ABI:

| Library | Functions | Location |
|---|---|---|
| `libmarketdata.so` | `tick_generator_create/destroy/next` | `market-data-svc/include/marketdata.h` |
| `libriskcalc.so` | `calc_mtm_pnl`, `calc_net_exposure`, `check_limit_breach` | `risk-svc/include/riskcalc.h` |

To replace either library with Rust:

```rust
#[no_mangle]
pub extern "C" fn calc_mtm_pnl(net_mw: f64, avg_fill: f64, lmp: f64) -> f64 {
    net_mw * (lmp - avg_fill)
}
```

Build as `cdylib`, drop in as the replacement `.so`. Go CGo and Python ctypes consumers require zero changes.

---

## Repo Layout

```
trade_infra/
├── market-data-svc/    C++17: LMP simulator (libmarketdata.so) + DB writer
├── order-svc/          Go: bid lifecycle, LISTEN/NOTIFY, REST API
├── risk-svc/           C++ libriskcalc.so + Go CGo: P&L, exposure, limits
├── strategy-engine/    Go: signal gate, order submission, metrics
├── python/
│   ├── strategies/     mean_reversion.py, ma_crossover.py, base.py
│   ├── data_gen.py     synthetic LMP seeder
│   ├── analytics.py    P&L + position CLI
│   └── smoke_test.py   integration test
├── sql/schema.sql      PostgreSQL schema + strategy_configs seed
├── infra/              docker-compose.yml, prometheus.yml, Grafana dashboard
└── .github/workflows/  per-service CI + integration pipeline
```
