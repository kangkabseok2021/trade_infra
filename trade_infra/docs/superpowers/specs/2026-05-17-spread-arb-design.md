# Spread-Arb Strategy Design

**Date:** 2026-05-17
**Version:** v0.6.0

## Goal

Add a cross-node spread-arbitrage strategy that trades the LMP price spread between HB_NORTH and HB_SOUTH using a rolling z-score signal.

## Context

`trade_infra` v0.5.0 has three single-node strategies (mean_reversion × 3 nodes, ma_crossover × 3 nodes). All strategies listen to one node's `price_ticks` NOTIFY channel, compute a signal, and emit rows into the `signals` table. The strategy-engine (Go) picks up signals, applies a per-node risk gate (position limit + cooldown), and POSTs orders to order-svc.

## Architecture

### New files

| File | Purpose |
|------|---------|
| `python/strategies/spread_arb.py` | New strategy: z-score on NORTH–SOUTH LMP spread |
| `python/tests/test_spread_arb.py` | Unit tests for `compute_spread_signal` |

### Modified files

| File | Change |
|------|--------|
| `python/strategies/base.py` | Add `listen_ticks_multi(db_url, nodes)` |
| `sql/schema.sql` | 6 new `strategy_configs` rows for `spread_arb` |
| `infra/docker-compose.yml` | 1 new service `spread-arb` |

### No changes to

strategy-engine, order-svc, risk-svc, market-data-svc, Prometheus config, Grafana dashboards.

## Data Flow

```
market-data-svc-north  ──NOTIFY price_ticks──┐
market-data-svc-south  ──NOTIFY price_ticks──┤
                                              ↓
                              spread_arb.py (LISTEN price_ticks)
                              ├── latest_lmp["HB_NORTH"] = lmp
                              ├── latest_lmp["HB_SOUTH"] = lmp
                              ├── spread = lmp_NORTH - lmp_SOUTH
                              ├── deque.append(spread)  (maxlen=window)
                              └── if |z-score| > threshold:
                                    emit_signal(NORTH, BUY/SELL)
                                    emit_signal(SOUTH, SELL/BUY)
                                           │
                                    NOTIFY 'signals'
                                           ↓
                              strategy-engine (Go)
                              ├── gate: position limit + cooldown per node
                              └── POST /orders to order-svc (one per leg)
```

## Signal Logic

```python
spread = lmp_NORTH - lmp_SOUTH
# Appended to rolling deque only when both nodes have at least one price

mean = statistics.mean(deque)
std  = statistics.stdev(deque)   # requires len >= 2
z    = (spread[-1] - mean) / std

z < -threshold  →  NORTH cheap vs SOUTH  →  ("BUY",  "SELL")   # buy NORTH, sell SOUTH
z > +threshold  →  NORTH expensive       →  ("SELL", "BUY")    # sell NORTH, buy SOUTH
otherwise       →  None
```

Guards: return `None` if `len(deque) < window`, or `std == 0`.

A spread value is only appended when **both** `latest_lmp["HB_NORTH"]` and `latest_lmp["HB_SOUTH"]` are populated (i.e., at least one tick has arrived from each node). This prevents computing a spread from a stale price if one node is offline.

## `listen_ticks_multi`

Added to `base.py`:

```python
def listen_ticks_multi(db_url: str, nodes: set[str]):
    """Yield tick dicts {node, lmp} for any node in the given set."""
    conn = psycopg2.connect(db_url)
    conn.set_isolation_level(psycopg2.extensions.ISOLATION_LEVEL_AUTOCOMMIT)
    cur = conn.cursor()
    cur.execute("LISTEN price_ticks")
    while True:
        if select.select([conn], [], [], 5.0) == ([], [], []):
            continue
        conn.poll()
        while conn.notifies:
            notify = conn.notifies.pop(0)
            try:
                payload = json.loads(notify.payload)
            except json.JSONDecodeError:
                continue
            if payload.get("node") in nodes:
                yield payload
```

The existing `price_ticks` NOTIFY channel carries the node in the payload — no new channel required.

## Strategy Config

New `strategy_configs` rows (added to `sql/schema.sql`):

| strategy | node | param_key | param_value |
|----------|------|-----------|-------------|
| spread_arb | HB_NORTH | window | 20 |
| spread_arb | HB_NORTH | threshold | 1.5 |
| spread_arb | HB_NORTH | quantity_mw | 5.0 |
| spread_arb | HB_SOUTH | window | 20 |
| spread_arb | HB_SOUTH | threshold | 1.5 |
| spread_arb | HB_SOUTH | quantity_mw | 5.0 |

`spread_arb.py` loads config for `node_a` (params used for both legs — window and threshold are spread-level, quantity_mw applies to each leg independently).

## Docker Compose

```yaml
spread-arb:
  build:
    context: ../python
    dockerfile: Dockerfile.strategy
  command: uv run python strategies/spread_arb.py --node-a HB_NORTH --node-b HB_SOUTH
  environment:
    DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
  depends_on:
    postgres:
      condition: service_healthy
  restart: unless-stopped
```

`spread_arb.py` uses `argparse` with `--node-a` / `--node-b` flags (same CLI pattern as existing strategies). `DATABASE_URL` is the only env var, matching the existing services.

## Testing

**`python/tests/test_spread_arb.py`** — unit tests for `compute_spread_signal(spreads, window, threshold)`:

| Test | Input | Expected |
|------|-------|----------|
| Below window | `len(spreads) < window` | `None` |
| Flat spread | all values equal (std=0) | `None` |
| Within band | z-score between ±threshold | `None` |
| Below band | z < -threshold | `("BUY", "SELL")` |
| Above band | z > +threshold | `("SELL", "BUY")` |

No DB integration tests needed — `emit_signal` and `listen_ticks_multi` DB paths are already covered by the existing strategy tests and the running Docker stack.

## Scope Boundaries

- No Prometheus metrics endpoint for spread-arb (signals and orders already tracked via `strategy` label on existing panels)
- No changes to strategy-engine (per-node cooldown handles leg independence)
- No new signal type (existing `signals` schema is sufficient)
- Single node pair (HB_NORTH / HB_SOUTH) — additional pairs are a config-only addition later

## Test Count Impact

Current: 49 Python tests (22 strategy-engine Python, existing). Adding ~5 new unit tests for `compute_spread_signal`. Total after: ~54 Python tests.
