# trade_infra

Energy trading infrastructure covering the full daily workflow of a trading system developer: market data simulation, order management, and risk/P&L monitoring — wired through PostgreSQL and observable via Prometheus + Grafana.

---

## Architecture

```
Python data_gen.py
    │  seeds synthetic ERCOT-style LMP history
    ▼
PostgreSQL
    │  NOTIFY 'price_ticks'
    ▼
market-data-svc  (C++17)
    │  Ornstein-Uhlenbeck LMP simulation → price_ticks table
    │  NOTIFY 'price_ticks' with JSON payload
    ▼
order-svc  (Go)
    │  LISTEN 'price_ticks' → evaluate PENDING orders vs current LMP
    │  fills BUY when lmp ≤ limit, SELL when lmp ≥ limit
    │  NOTIFY 'order_updates' with fill details
    ▼
risk-svc  (C++ libriskcalc.so + Go CGo)
    │  LISTEN 'order_updates' → MTM P&L, net exposure, limit breach
    │  writes risk_snapshots to PostgreSQL
    ▼
Prometheus  ──scrapes /metrics──►  all three services
    ▼
Grafana  ──dashboards──►  tick rate, fill count, latency p99, availability
```

### Services

| Service | Language | Port (API / Metrics) | Responsibility |
|---|---|---|---|
| market-data-svc | C++17 + libpq | — / 9101 | LMP tick simulation, PostgreSQL NOTIFY |
| order-svc | Go 1.22 | 8080 / 9102 | Bid lifecycle, LISTEN/NOTIFY, REST API |
| risk-svc | C++ (CGo) + Go | 8081 / 9103 | MTM P&L, exposure, limit breach |

### Database tables

| Table | Purpose |
|---|---|
| `price_ticks` | LMP tick history per node |
| `orders` | Bid lifecycle (PENDING → FILLED / REJECTED) |
| `positions` | Net MW and average fill price per node |
| `risk_snapshots` | MTM P&L, exposure, limit headroom per fill |

---

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| CMake | ≥ 3.20 | `brew install cmake` |
| C++ compiler | C++17 | Xcode CLT / `apt-get install build-essential` |
| libpq (PostgreSQL C client) | any | `brew install postgresql` / `apt-get install libpq-dev` |
| Go | ≥ 1.22 | [go.dev/dl](https://go.dev/dl) |
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
uv run python data_gen.py --nodes HB_NORTH,HB_SOUTH --ticks 3600
```

### 3. market-data-svc

```bash
cd market-data-svc
cmake -B build -S . && cmake --build build --target market_data_svc -j4
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra" \
NODE_NAME=HB_NORTH INTERVAL_MS=1000 \
./build/market_data_svc
```

### 4. order-svc

```bash
cd order-svc
DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable" \
go run ./cmd/server
```

Submit an order:

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

Check risk snapshot:

```bash
curl -s http://localhost:8081/risk/HB_NORTH | python3 -m json.tool
```

### 6. Analytics

```bash
cd python
uv run python analytics.py --db-url "postgresql://$(whoami)@localhost:5432/trade_infra?sslmode=disable"
```

---

## Docker Compose

Starts all services + PostgreSQL + Prometheus + Grafana:

```bash
cd infra
docker compose up --build
```

| Service | URL |
|---|---|
| order-svc REST | http://localhost:8080 |
| risk-svc REST | http://localhost:8081 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 (admin / admin) |

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
# order-svc — 8 tests (requires trade_infra_test DB)
cd order-svc
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v

# risk-svc — 6 tests (requires libriskcalc.so + trade_infra_test DB)
cd risk-svc
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v
```

### Python (pytest + ruff)

```bash
cd python
uv sync --extra dev
uv run ruff check .          # lint
uv run pytest tests/ -v      # 9 tests
```

---

## Monitoring

Prometheus scrapes each service's `/metrics` endpoint every 15 seconds.

| Metric | Service | Description |
|---|---|---|
| `market_data_tick_total` | market-data-svc | Price ticks written |
| `order_svc_orders_created_total` | order-svc | Orders submitted |
| `order_svc_orders_filled_total` | order-svc | Orders filled |
| `order_svc_eval_latency_seconds` | order-svc | Evaluation latency histogram |
| `risk_svc_snapshots_saved_total` | risk-svc | Risk snapshots written |
| `risk_svc_limit_breaches_total` | risk-svc | Position limit breaches |

### SLOs

| SLO | Target | Alert |
|---|---|---|
| Order evaluation latency p99 | < 50ms | `OrderEvalLatencyHigh` (warning, 2m) |
| All services up | 100% | `ServiceDown` (critical, 1m) |

---

## Rust Migration Path

Every C++ computation is isolated in a shared library with a clean `extern "C"` ABI:

| Library | Functions | Location |
|---|---|---|
| `libmarketdata.so` | `tick_generator_create/destroy/next` | `market-data-svc/include/marketdata.h` |
| `libriskcalc.so` | `calc_mtm_pnl`, `calc_net_exposure`, `check_limit_breach` | `risk-svc/include/riskcalc.h` |

To replace either library with Rust:

```rust
// In a Rust cdylib crate — expose the same symbols:
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
├── market-data-svc/     C++17: LMP simulator + PostgreSQL writer
├── order-svc/           Go: bid lifecycle, LISTEN/NOTIFY, REST API
├── risk-svc/            C++ (CGo) + Go: P&L, exposure, limit breach
├── python/              uv: data_gen.py, analytics.py, smoke_test.py
├── sql/schema.sql       PostgreSQL schema
├── infra/               docker-compose.yml, prometheus.yml, Grafana JSON
└── .github/workflows/   per-service CI + integration pipeline
```
