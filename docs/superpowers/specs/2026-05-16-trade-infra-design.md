# trade_infra — System Design

**Date:** 2026-05-16
**Domain:** Energy markets (ERCOT-style LMP simulation)
**Stack:** C++, Go, Python (uv), PostgreSQL, Prometheus, Grafana
**Repo:** git@github.com:kangkabseok2021/trade_infra.git

---

## 1. Goal

Build a realistic energy trading infrastructure that covers three daily concerns of a trading system developer:

1. **Market data pipeline** — synthetic LMP price tick simulation
2. **Order/bid management** — full bid lifecycle from submission to fill/rejection
3. **Risk & P&L monitoring** — real-time mark-to-market, exposure limits, SLO dashboards

Built in phases so each phase ships a working vertical slice. Designed from day one for a future Rust migration: all C++ computation is isolated in shared libraries with clean `extern "C"` ABI.

---

## 2. Architecture

Three independent services communicate via PostgreSQL `LISTEN/NOTIFY`. Prometheus scrapes all services. Grafana provides dashboards and SLO burn-rate views.

```
Python data_gen.py
    │ seeds historical prices + synthetic nodes
    ▼
PostgreSQL: price_ticks
    │ NOTIFY 'price_ticks'
    ▼
market-data-svc (C++ libmarketdata.so)
    │ generates real-time tick stream
    │ writes to price_ticks
    │ NOTIFY 'price_ticks'
    ▼
order-svc (Go)
    │ LISTEN 'price_ticks'
    │ evaluates PENDING orders against current LMP
    │ writes fills/rejections to orders table
    │ NOTIFY 'order_updates'
    ▼
risk-svc (C++ libriskcalc.so + Go)
    │ LISTEN 'order_updates'
    │ recalculates P&L, net exposure per node
    │ checks limit breaches
    │ writes to risk_snapshots
    │ NOTIFY 'risk_alerts' on breach
    ▼
Prometheus scrapes /metrics (all services)
    ▼
Grafana dashboard
```

---

## 3. Components

### market-data-svc (C++)

- Shared library: `libmarketdata.so` with `extern "C"` ABI
- Thin C++ binary wrapper
- Simulates ERCOT-style LMP, load forecast, generation mix per node
- Writes tick records to `price_ticks` at configurable interval (default 1s)
- Embedded HTTP server via `cpp-httplib` for `/metrics`
- Build: CMake

### order-svc (Go)

- Full bid lifecycle: `PENDING → SUBMITTED → PARTIALLY_FILLED → FILLED | REJECTED | CANCELLED`
- `LISTEN 'price_ticks'` — evaluates open orders against current LMP
- REST API: submit order, query order status, list open orders
- `/metrics` via Go Prometheus client
- Build: `go build`

### risk-svc (C++ + Go)

- Shared library: `libriskcalc.so` with `extern "C"` ABI (mark-to-market P&L, net exposure, limit breach detection)
- Go binary wraps it via CGo, exposes REST API + `/metrics`
- `LISTEN 'order_updates'` — reacts to fills in real-time
- Writes risk snapshots to `risk_snapshots`
- Build: CMake (C++) + `go build` (Go wrapper)

### Python tools

- Managed with `uv` (`pyproject.toml`)
- `data_gen.py` — seeds synthetic price history, bootstraps simulation
- `analytics.py` — queries PostgreSQL, prints position summaries, P&L reports, SLO compliance
- Test runner: `uv run pytest`

### Monitoring

- **Prometheus** — scrapes `/metrics` on all three services
- **Grafana** — pre-built dashboard JSON (committed to repo): tick ingestion rate, order fill latency, risk limit headroom, SLO burn rate
- Both run as Docker Compose services

### CI/CD (GitHub Actions)

- Per-service jobs: C++ (CMake), Go (`go test ./...`), Python (`ruff` + `pytest`)
- Integration job: `docker compose up` → smoke test → tear down
- All jobs must pass before merge to `main`
- Remote: `git@github.com:kangkabseok2021/trade_infra.git`

---

## 4. Database Schema

```sql
-- PostgreSQL

CREATE TABLE price_ticks (
    id          BIGSERIAL PRIMARY KEY,
    node        TEXT NOT NULL,
    lmp         NUMERIC(10,4) NOT NULL,   -- $/MWh
    load_mw     NUMERIC(10,2),
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          BIGSERIAL PRIMARY KEY,
    node        TEXT NOT NULL,
    side        TEXT NOT NULL CHECK (side IN ('BUY','SELL')),
    quantity_mw NUMERIC(10,2) NOT NULL,
    limit_price NUMERIC(10,4) NOT NULL,
    status      TEXT NOT NULL DEFAULT 'PENDING',
    filled_at   NUMERIC(10,4),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE positions (
    id          BIGSERIAL PRIMARY KEY,
    node        TEXT NOT NULL,
    net_mw      NUMERIC(10,2) NOT NULL DEFAULT 0,
    avg_price   NUMERIC(10,4),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE risk_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    node            TEXT NOT NULL,
    mtm_pnl         NUMERIC(14,4) NOT NULL,
    net_exposure_mw NUMERIC(10,2) NOT NULL,
    limit_headroom  NUMERIC(10,2) NOT NULL,
    snapshot_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## 5. Repository Layout

```
trade_infra/
├── market-data-svc/        # C++ CMake project
│   ├── CMakeLists.txt
│   ├── src/
│   └── include/
├── order-svc/              # Go module
│   ├── go.mod
│   └── cmd/server/
├── risk-svc/               # C++ CMake + Go CGo wrapper
│   ├── CMakeLists.txt
│   ├── src/
│   └── cmd/server/
├── python/                 # uv project
│   ├── pyproject.toml
│   ├── data_gen.py
│   └── analytics.py
├── infra/
│   ├── docker-compose.yml
│   ├── prometheus.yml
│   └── grafana/dashboards/
├── sql/
│   └── schema.sql
└── .github/workflows/
    ├── market-data-svc.yml
    ├── order-svc.yml
    ├── risk-svc.yml
    └── integration.yml
```

---

## 6. Error Handling

| Scenario | Handling |
|---|---|
| `market-data-svc` crash | Docker restarts; missed ticks are synthetic — no data loss |
| `order-svc` loses DB connection | Exponential backoff reconnect; order state persisted in PostgreSQL |
| Risk limit breach | Written to `risk_snapshots`, Prometheus counter incremented; `order-svc` checks headroom before accepting orders |
| PostgreSQL down | All services fail fast; Docker Compose health checks gate startup |

---

## 7. Testing

| Layer | Tool | Coverage |
|---|---|---|
| C++ unit | GoogleTest (CMake) | `libmarketdata.so` tick gen, `libriskcalc.so` P&L math |
| Go unit | `go test ./...` | Order state machine, REST handler logic |
| Python | `uv run pytest` | data_gen determinism, analytics query output |
| Integration | Python smoke test | Full compose stack → generate → order → verify risk snapshot in DB |

---

## 8. SLO Definitions (Prometheus Recording Rules)

| SLO | Target |
|---|---|
| Tick ingestion latency p99 | < 100ms |
| Order evaluation latency p99 | < 50ms |
| Risk snapshot staleness after fill | < 5s |
| Service availability (all three) | > 99.5% over 7-day window |

---

## 9. Build Order (Phases)

| Phase | Deliverable |
|---|---|
| 1 | PostgreSQL schema + Python data generator + `market-data-svc` tick simulator |
| 2 | `order-svc` — bid lifecycle, LISTEN/NOTIFY, REST API |
| 3 | `risk-svc` — C++ P&L calcs + Go HTTP API + Grafana dashboard |
| 4 | GitHub Actions CI/CD, Docker Compose, integration tests, SLO alerting |

---

## 10. Rust Migration Path

Each C++ shared library (`libmarketdata.so`, `libriskcalc.so`) uses a clean `extern "C"` ABI. To migrate a service to Rust:

1. Implement equivalent logic in a Rust crate
2. Expose the same `extern "C"` symbols via `#[no_mangle]`
3. Build as `cdylib` and drop in as replacement `.so`
4. Go CGo and Python ctypes consumers require zero changes

No flag days. Migrate one library at a time.
