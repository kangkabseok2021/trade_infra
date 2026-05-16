# Multi-Node Support — System Design

**Date:** 2026-05-16
**Parent system:** trade_infra
**Scope:** Add HB_SOUTH and HB_WEST alongside existing HB_NORTH

---

## 1. Goal

Extend trade_infra from one ERCOT node (HB_NORTH) to three (HB_NORTH, HB_SOUTH, HB_WEST). Each node runs independent market data simulation, independent strategy daemons, and independent position tracking. Per-node position limits are enforced by the existing gate (already queries by node) and by risk-svc (limit made configurable).

---

## 2. Approach

**Zero logic changes to any service binary except one small parameterisation in risk-svc.**

The existing architecture already supports multiple nodes:
- `market-data-svc` takes `NODE_NAME` env var — run one container per node
- Strategy daemons take `--node` CLI arg — run one container per node per strategy
- `order-svc` and `risk-svc` are node-agnostic (all DB queries filter by node)
- `strategy-engine` gate queries `net_exposure_mw` per node via `risk_snapshots WHERE node=$1`

Changes required:
1. **sql/schema.sql** — seed 12 new `strategy_configs` rows for HB_SOUTH and HB_WEST
2. **risk-svc** — change `positionLimitMW = 50.0` hardcoded constant to read `RISK_POSITION_LIMIT_MW` env var (default 50.0)
3. **infra/docker-compose.yml** — add 6 new service blocks
4. **infra/prometheus.yml** — add 2 scrape targets

---

## 3. Node Parameters

| Node | base_lmp | volatility | Notes |
|---|---|---|---|
| HB_NORTH | 45.0 | 5.0 | existing |
| HB_SOUTH | 42.0 | 7.0 | slightly cheaper, higher vol |
| HB_WEST | 38.0 | 6.0 | lowest price, moderate vol |

---

## 4. Strategy Config Seed Rows

Appended to `sql/schema.sql` with `ON CONFLICT DO NOTHING`:

```sql
-- HB_SOUTH
('mean_reversion', 'HB_SOUTH', 'window',       '20'),
('mean_reversion', 'HB_SOUTH', 'threshold',    '1.0'),
('mean_reversion', 'HB_SOUTH', 'quantity_mw',  '5.0'),
('ma_crossover',   'HB_SOUTH', 'fast_period',  '5'),
('ma_crossover',   'HB_SOUTH', 'slow_period',  '20'),
('ma_crossover',   'HB_SOUTH', 'quantity_mw',  '5.0'),
-- HB_WEST (higher threshold + smaller qty — lower volatility node)
('mean_reversion', 'HB_WEST',  'window',       '20'),
('mean_reversion', 'HB_WEST',  'threshold',    '1.2'),
('mean_reversion', 'HB_WEST',  'quantity_mw',  '4.0'),
('ma_crossover',   'HB_WEST',  'fast_period',  '5'),
('ma_crossover',   'HB_WEST',  'slow_period',  '20'),
('ma_crossover',   'HB_WEST',  'quantity_mw',  '4.0')
```

HB_WEST uses `threshold=1.2` (vs 1.0) to account for lower absolute volatility, and `quantity_mw=4.0` for smaller position sizing — demonstrating per-node param tuning.

---

## 5. risk-svc Change

**File:** `risk-svc/internal/listener/listener.go`

Change:
```go
const positionLimitMW = 50.0
```

To:
```go
var positionLimitMW = func() float64 {
    if v := os.Getenv("RISK_POSITION_LIMIT_MW"); v != "" {
        if f, err := strconv.ParseFloat(v, 64); err == nil {
            return f
        }
    }
    return 50.0
}()
```

Add `"os"` and `"strconv"` to imports. Add `RISK_POSITION_LIMIT_MW: "50"` to docker-compose.yml risk-svc environment block for explicitness.

The limit is the same value for all nodes (per-node semantics come from `risk_snapshots WHERE node=$1` — each node's exposure is checked independently).

---

## 6. Docker Compose Additions

6 new service blocks added to `infra/docker-compose.yml`:

```yaml
  market-data-svc-south:
    build: {context: ../market-data-svc, dockerfile: Dockerfile}
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_SOUTH
      BASE_LMP: "42.0"
      VOLATILITY: "7.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
    depends_on:
      postgres: {condition: service_healthy}

  market-data-svc-west:
    build: {context: ../market-data-svc, dockerfile: Dockerfile}
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_WEST
      BASE_LMP: "38.0"
      VOLATILITY: "6.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
    depends_on:
      postgres: {condition: service_healthy}

  mean-reversion-south:
    build: {context: ../python, dockerfile: Dockerfile.strategy}
    command: uv run python strategies/mean_reversion.py --node HB_SOUTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres: {condition: service_healthy}
    restart: unless-stopped

  mean-reversion-west:
    build: {context: ../python, dockerfile: Dockerfile.strategy}
    command: uv run python strategies/mean_reversion.py --node HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres: {condition: service_healthy}
    restart: unless-stopped

  ma-crossover-south:
    build: {context: ../python, dockerfile: Dockerfile.strategy}
    command: uv run python strategies/ma_crossover.py --node HB_SOUTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres: {condition: service_healthy}
    restart: unless-stopped

  ma-crossover-west:
    build: {context: ../python, dockerfile: Dockerfile.strategy}
    command: uv run python strategies/ma_crossover.py --node HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres: {condition: service_healthy}
    restart: unless-stopped
```

New market-data-svc instances use the same internal METRICS_PORT (9101) — no host port conflict since they're separate Docker services. Only the original market-data-svc exposes 9101 externally.

---

## 7. Prometheus Additions

Append to `scrape_configs` in `infra/prometheus.yml`:

```yaml
  - job_name: market-data-svc-south
    static_configs:
      - targets: ["market-data-svc-south:9101"]

  - job_name: market-data-svc-west
    static_configs:
      - targets: ["market-data-svc-west:9101"]
```

---

## 8. Error Handling

| Scenario | Handling |
|---|---|
| One market-data-svc crashes | Other nodes unaffected; Docker Compose restarts it |
| Strategy daemon crashes for one node | `restart: unless-stopped` recovers; warm-up re-runs harmlessly |
| Missing strategy_configs for a node | Strategy crashes at startup with clear `KeyError`; visible in `docker compose logs` |

---

## 9. Verification

After `docker compose up --build`:

```sql
-- All 3 nodes producing ticks
SELECT node, COUNT(*) FROM price_ticks GROUP BY node ORDER BY node;

-- Signals from all 3 nodes
SELECT node, strategy, COUNT(*) FROM signals GROUP BY node, strategy ORDER BY node;

-- Positions per node
SELECT node, net_mw, avg_price FROM positions ORDER BY node;
```

---

## 10. Build Order

| Step | Change |
|---|---|
| 1 | Append 12 seed rows to `sql/schema.sql` |
| 2 | Make `positionLimitMW` configurable in `risk-svc/internal/listener/listener.go` |
| 3 | Add `RISK_POSITION_LIMIT_MW: "50"` to risk-svc env in `docker-compose.yml` |
| 4 | Add 6 service blocks to `docker-compose.yml` |
| 5 | Add 2 scrape targets to `prometheus.yml` |
| 6 | `docker compose up --build` and run verification queries |

---

## 11. No Changes To

- order-svc (node-agnostic)
- strategy-engine gate (already per-node via `LatestNetExposure(node)`)
- market-data-svc C++ code
- Python strategy logic
- Any test files (existing 49 tests pass unchanged)
