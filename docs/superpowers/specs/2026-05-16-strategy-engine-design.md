# Strategy Engine — System Design

**Date:** 2026-05-16
**Parent system:** trade_infra
**Repo:** git@github.com:kangkabseok2021/trade_infra.git

---

## 1. Goal

Add automated trading to trade_infra: Python strategy daemons analyse price ticks and write signals to PostgreSQL; a Go strategy engine consumes signals, applies risk gating, and submits orders to order-svc. Phase 1 implements two strategies: mean reversion and moving average crossover.

---

## 2. Architecture

```
market-data-svc
    │  NOTIFY 'price_ticks'
    ▼
PostgreSQL: price_ticks
    │
    ├──► mean_reversion.py       (Python daemon)
    │       LISTEN 'price_ticks' → rolling window → NOTIFY 'signals'
    │
    └──► ma_crossover.py         (Python daemon)
            LISTEN 'price_ticks' → fast/slow EMA → NOTIFY 'signals'
    │
    ▼
PostgreSQL: signals
    │  NOTIFY 'signals'
    ▼
strategy-engine                  (Go service)
    │  LISTEN 'signals'
    │  risk gate (position limit + cooldown)
    │  POST /orders → order-svc
    │  /metrics :9104
    ▼
order-svc → fills → risk-svc     (unchanged)
```

---

## 3. New Database Tables

```sql
CREATE TABLE signals (
    id           BIGSERIAL PRIMARY KEY,
    strategy     TEXT NOT NULL,
    node         TEXT NOT NULL,
    side         TEXT NOT NULL CHECK (side IN ('BUY','SELL')),
    quantity_mw  NUMERIC(10,2) NOT NULL,
    limit_price  NUMERIC(10,4) NOT NULL,
    status       TEXT NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING','SUBMITTED','SKIPPED')),
    reason       TEXT,
    order_id     BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_signals_strategy_node_ts ON signals (strategy, node, created_at DESC);

CREATE TABLE strategy_configs (
    strategy    TEXT NOT NULL,
    node        TEXT NOT NULL,
    param_key   TEXT NOT NULL,
    param_value TEXT NOT NULL,
    PRIMARY KEY (strategy, node, param_key)
);
```

**Default config rows (seeded at startup):**

| strategy | node | param_key | param_value |
|---|---|---|---|
| mean_reversion | HB_NORTH | window | 20 |
| mean_reversion | HB_NORTH | threshold | 1.0 |
| mean_reversion | HB_NORTH | quantity_mw | 5.0 |
| ma_crossover | HB_NORTH | fast_period | 5 |
| ma_crossover | HB_NORTH | slow_period | 20 |
| ma_crossover | HB_NORTH | quantity_mw | 5.0 |

---

## 4. Components

### mean_reversion.py

Maintains a rolling deque of the last `window` LMP values (loaded from `strategy_configs`). On each tick:

- If fewer than `window` ticks buffered: skip (warm-up).
- Compute rolling mean and std dev.
- BUY signal when `lmp < mean - threshold * std` (price cheap relative to recent history).
- SELL signal when `lmp > mean + threshold * std` (price expensive).
- Otherwise: no action.

On signal: `INSERT INTO signals`, `NOTIFY 'signals'`.

### ma_crossover.py

Maintains two EMAs with configurable `fast_period` and `slow_period`. EMA update: `ema = α * lmp + (1-α) * prev_ema`, where `α = 2 / (period + 1)`.

- BUY signal on the tick where fast EMA crosses **above** slow EMA (edge detection only, not sustained level).
- SELL signal on the tick where fast EMA crosses **below** slow EMA.
- After `slow_period` ticks of warm-up: no signals during initialisation.

On signal: `INSERT INTO signals`, `NOTIFY 'signals'`.

### strategy-engine (Go)

**Signal consumer:** `LISTEN 'signals'`. On NOTIFY, `SELECT` the pending signal row. Before any action: `UPDATE signals SET status='SUBMITTED'` (or `'SKIPPED'`) atomically — idempotent against restarts.

**Risk gate (checked before submitting):**
1. **Position limit** — query latest `risk_snapshots` for node; if `net_exposure_mw + quantity_mw >= position_limit_mw` (env var `POSITION_LIMIT_MW`, default 40), skip with reason `risk_limit`.
2. **Cooldown** — query `MAX(created_at) FROM signals WHERE strategy=$1 AND node=$2 AND status='SUBMITTED'`; if within `COOLDOWN_SECS` (env var, default 30), skip with reason `cooldown`.

**Order submission:** `POST http://order-svc:8080/orders`. On success: `UPDATE signals SET order_id=<id>`. On HTTP error: log, leave status as SUBMITTED (no retry — avoids double-submit).

**Startup:** exponential backoff retry until order-svc `/health` returns 200.

**Metrics (`/metrics` on `:9104`):**
- `strategy_engine_signals_received_total{strategy,node}`
- `strategy_engine_signals_submitted_total{strategy,node}`
- `strategy_engine_signals_skipped_total{strategy,node,reason}`

---

## 5. Repository Layout

```
trade_infra/
├── strategy-engine/             Go service
│   ├── go.mod
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── signal/
│   │   │   ├── model.go
│   │   │   ├── store.go
│   │   │   └── store_test.go
│   │   ├── gate/
│   │   │   ├── gate.go
│   │   │   └── gate_test.go
│   │   ├── listener/
│   │   │   └── listener.go
│   │   └── submitter/
│   │       ├── submitter.go
│   │       └── submitter_test.go
│   └── metrics/metrics.go
├── python/
│   ├── strategies/
│   │   ├── base.py              shared LISTEN/INSERT helpers
│   │   ├── mean_reversion.py
│   │   └── ma_crossover.py
│   └── tests/
│       ├── test_mean_reversion.py
│       └── test_ma_crossover.py
└── sql/schema.sql               (append signals + strategy_configs tables)
```

---

## 6. Data Flow

```
1. market-data-svc writes tick → NOTIFY 'price_ticks'
2. Strategy daemon receives NOTIFY, updates in-memory window
3. If signal fires:
     INSERT INTO signals (strategy, node, side, quantity_mw,
                          limit_price, status='PENDING')
     NOTIFY 'signals'
4. strategy-engine receives NOTIFY
5. UPDATE signals SET status='SUBMITTED'/'SKIPPED', reason=...
   (atomic before HTTP call — idempotent on restart)
6. If SUBMITTED: POST /orders to order-svc
                 UPDATE signals SET order_id=<returned id>
7. order-svc fills → NOTIFY 'order_updates' → risk-svc (unchanged)
```

---

## 7. Error Handling

| Scenario | Handling |
|---|---|
| Strategy daemon crash | Docker Compose restarts; re-subscribes to price_ticks; warm-up window re-runs from latest ticks |
| `POST /orders` fails | Log error; signal stays SUBMITTED; no retry (prevents double-submit) |
| order-svc unavailable at startup | Exponential backoff on `/health` check before starting LISTEN loop |
| Duplicate NOTIFY (PostgreSQL at-least-once) | Idempotent: `SELECT FOR UPDATE` on signal row; already-SUBMITTED rows are no-ops |

---

## 8. Testing

| Layer | Tool | Coverage |
|---|---|---|
| Python unit | `pytest` | Signal logic with synthetic tick sequences; verify exact tick of signal, warm-up period, edge detection, no double-signal |
| Go unit | `go test` | Risk gate: limit check, cooldown logic; signal store CRUD; submitter HTTP mock |
| Integration | extend smoke_test.py | Seed ticks matching mean-reversion pattern; wait for signal row; verify order submitted |

---

## 9. Docker Compose additions

```yaml
mean-reversion:
  build: {context: ../python, dockerfile: Dockerfile.strategy}
  command: uv run python strategies/mean_reversion.py --node HB_NORTH
  environment:
    DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
  depends_on:
    postgres: {condition: service_healthy}

ma-crossover:
  build: {context: ../python, dockerfile: Dockerfile.strategy}
  command: uv run python strategies/ma_crossover.py --node HB_NORTH
  environment:
    DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
  depends_on:
    postgres: {condition: service_healthy}

strategy-engine:
  build: {context: ../strategy-engine, dockerfile: Dockerfile}
  environment:
    DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    ORDER_SVC_URL: http://order-svc:8080
    POSITION_LIMIT_MW: "40"
    COOLDOWN_SECS: "30"
    METRICS_ADDR: ":9104"
  ports: ["9104:9104"]
  depends_on:
    postgres: {condition: service_healthy}
    order-svc: {condition: service_started}
```

---

## 10. Build Order

| Phase | Deliverable |
|---|---|
| 1 | SQL schema additions + strategy_configs seed |
| 2 | Python base.py + mean_reversion.py (TDD) |
| 3 | Python ma_crossover.py (TDD) |
| 4 | strategy-engine Go: signal model + store (TDD) |
| 5 | strategy-engine Go: risk gate (TDD) |
| 6 | strategy-engine Go: listener + submitter + main |
| 7 | Docker Compose additions + Prometheus scrape config |
| 8 | Integration smoke test extension |
