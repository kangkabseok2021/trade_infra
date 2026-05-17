# Spread-Arb Strategy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a cross-node spread-arb strategy that trades the HB_NORTH/HB_SOUTH LMP spread on z-score signal, using the existing LISTEN/NOTIFY pipeline and strategy-engine risk gate.

**Architecture:** A new `listen_ticks_multi` helper in `base.py` yields ticks for any set of nodes from the single `price_ticks` NOTIFY channel. `spread_arb.py` maintains per-node latest prices and a rolling spread deque, calls the pure `compute_spread_signal` function, and emits two signals (one per leg) when the z-score crosses the threshold. A single new Docker Compose service runs the strategy with `--node-a HB_NORTH --node-b HB_SOUTH`.

**Tech Stack:** Python 3.12, psycopg2, `statistics` stdlib, `collections.deque`, uv, PostgreSQL LISTEN/NOTIFY, Docker Compose.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `python/strategies/base.py` | Modify | Add `listen_ticks_multi(db_url, nodes)` |
| `python/strategies/spread_arb.py` | Create | `compute_spread_signal` + `run` + argparse entrypoint |
| `python/tests/test_spread_arb.py` | Create | 5 unit tests for `compute_spread_signal` |
| `sql/schema.sql` | Modify | 6 new `strategy_configs` seed rows for `spread_arb` |
| `infra/docker-compose.yml` | Modify | 1 new `spread-arb` service |

---

## Task 1: Add `listen_ticks_multi` to `base.py`

**Files:**
- Modify: `python/strategies/base.py`

`listen_ticks_multi` is infrastructure (LISTEN/NOTIFY) — it can't be unit tested without a live DB and is verified in Task 6 via Docker. No unit test for this task.

- [ ] **Step 1: Open `python/strategies/base.py` and append `listen_ticks_multi` after `listen_ticks`**

The existing `listen_ticks` (line 19) filters by one node. `listen_ticks_multi` filters by a set.

Add this function at the end of `python/strategies/base.py`:

```python
def listen_ticks_multi(db_url: str, nodes: set[str]):
    """Yield tick dicts {node, lmp} for any node in the given set via LISTEN price_ticks."""
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

`select`, `json`, and `psycopg2` are already imported at the top of `base.py` — no new imports needed.

- [ ] **Step 2: Verify the file looks correct**

Run:
```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run python -c "from strategies.base import listen_ticks_multi; print('ok')"
```

Expected output:
```
ok
```

- [ ] **Step 3: Commit**

```bash
git add python/strategies/base.py
git commit -m "feat(spread-arb): add listen_ticks_multi to base.py"
git push origin main
```

---

## Task 2: Write failing tests for `compute_spread_signal`

**Files:**
- Create: `python/tests/test_spread_arb.py`

`compute_spread_signal` doesn't exist yet — the import will fail with `ModuleNotFoundError`. That is the expected failing state.

- [ ] **Step 1: Create `python/tests/test_spread_arb.py` with these 5 tests**

```python
import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.spread_arb import compute_spread_signal


def test_below_window_returns_none():
    # Only 5 spreads, window=20 — still warming up
    result = compute_spread_signal([5.0] * 5, window=20, threshold=1.5)
    assert result is None


def test_flat_spread_returns_none():
    # std=0 → guard against division by zero
    result = compute_spread_signal([3.0] * 20, window=20, threshold=1.5)
    assert result is None


def test_within_band_returns_none():
    # Alternating 0.0/2.0 → mean=1.0, std≈1.03, last z≈0.97 < 1.5
    spreads = [0.0, 2.0] * 10
    result = compute_spread_signal(spreads, window=20, threshold=1.5)
    assert result is None


def test_below_band_returns_buy_sell():
    # 19 spreads at 0.0, last at -20.0
    # mean=-1.0, std≈4.47, z≈-4.25 < -1.5 → BUY node_a, SELL node_b
    spreads = [0.0] * 19 + [-20.0]
    result = compute_spread_signal(spreads, window=20, threshold=1.5)
    assert result == ('BUY', 'SELL')


def test_above_band_returns_sell_buy():
    # 19 spreads at 0.0, last at +20.0
    # mean=1.0, std≈4.47, z≈+4.25 > 1.5 → SELL node_a, BUY node_b
    spreads = [0.0] * 19 + [20.0]
    result = compute_spread_signal(spreads, window=20, threshold=1.5)
    assert result == ('SELL', 'BUY')
```

- [ ] **Step 2: Run to confirm they fail with `ModuleNotFoundError`**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run pytest tests/test_spread_arb.py -v
```

Expected: all 5 tests fail with:
```
ModuleNotFoundError: No module named 'strategies.spread_arb'
```

- [ ] **Step 3: Commit**

```bash
git add python/tests/test_spread_arb.py
git commit -m "test(spread-arb): add 5 failing tests for compute_spread_signal"
git push origin main
```

---

## Task 3: Implement `spread_arb.py`

**Files:**
- Create: `python/strategies/spread_arb.py`

- [ ] **Step 1: Create `python/strategies/spread_arb.py`**

```python
import argparse
import statistics
from collections import deque

from strategies.base import emit_signal, listen_ticks_multi, load_config


def compute_spread_signal(
    spreads: list[float],
    window: int,
    threshold: float,
) -> tuple[str, str] | None:
    """Return (side_a, side_b) or None based on z-score of the rolling spread series.

    side_a/side_b are 'BUY' or 'SELL' for node_a and node_b respectively.
    """
    if len(spreads) < window:
        return None
    window_data = spreads[-window:]
    if len(window_data) < 2:
        return None
    mean = statistics.mean(window_data)
    std = statistics.stdev(window_data)
    if std == 0:
        return None
    z = (window_data[-1] - mean) / std
    if z < -threshold:
        return ('BUY', 'SELL')
    if z > threshold:
        return ('SELL', 'BUY')
    return None


def run(db_url: str, node_a: str, node_b: str) -> None:
    cfg_a = load_config(db_url, 'spread_arb', node_a)
    cfg_b = load_config(db_url, 'spread_arb', node_b)
    window = int(cfg_a['window'])
    threshold = float(cfg_a['threshold'])
    qty_a = float(cfg_a['quantity_mw'])
    qty_b = float(cfg_b['quantity_mw'])

    latest_lmp: dict[str, float] = {}
    spreads: deque[float] = deque(maxlen=window)

    print(f"spread_arb: node_a={node_a} node_b={node_b} window={window} threshold={threshold}")

    for tick in listen_ticks_multi(db_url, {node_a, node_b}):
        latest_lmp[tick['node']] = tick['lmp']
        if node_a not in latest_lmp or node_b not in latest_lmp:
            continue
        spread = latest_lmp[node_a] - latest_lmp[node_b]
        spreads.append(spread)
        result = compute_spread_signal(list(spreads), window, threshold)
        if result:
            side_a, side_b = result
            id_a = emit_signal(db_url, 'spread_arb', node_a, side_a, qty_a, latest_lmp[node_a])
            id_b = emit_signal(db_url, 'spread_arb', node_b, side_b, qty_b, latest_lmp[node_b])
            print(
                f"spread_arb: signal ids={id_a},{id_b} "
                f"{side_a} {node_a} / {side_b} {node_b}"
            )


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node-a', default='HB_NORTH')
    p.add_argument('--node-b', default='HB_SOUTH')
    p.add_argument('--db-url', default=os.getenv(
        'DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra',
    ))
    args = p.parse_args()
    run(args.db_url, args.node_a, args.node_b)
```

- [ ] **Step 2: Run the tests — all 5 should pass**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run pytest tests/test_spread_arb.py -v
```

Expected output:
```
tests/test_spread_arb.py::test_below_window_returns_none PASSED
tests/test_spread_arb.py::test_flat_spread_returns_none PASSED
tests/test_spread_arb.py::test_within_band_returns_none PASSED
tests/test_spread_arb.py::test_below_band_returns_buy_sell PASSED
tests/test_spread_arb.py::test_above_band_returns_sell_buy PASSED

5 passed
```

- [ ] **Step 3: Run the full Python test suite — no regressions**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run pytest tests/ -v
```

Expected: all tests pass (was 22 before; now 27).

- [ ] **Step 4: Commit**

```bash
git add python/strategies/spread_arb.py
git commit -m "feat(spread-arb): implement compute_spread_signal and run loop"
git push origin main
```

---

## Task 4: Add `strategy_configs` seed rows to `sql/schema.sql`

**Files:**
- Modify: `sql/schema.sql`

- [ ] **Step 1: Open `sql/schema.sql` and append a new INSERT block at the end of the file (after the existing `ON CONFLICT DO NOTHING;` on the last line)**

Add:

```sql
-- Spread-arb strategy config (HB_NORTH / HB_SOUTH pair)
INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('spread_arb', 'HB_NORTH', 'window',       '20'),
    ('spread_arb', 'HB_NORTH', 'threshold',    '1.5'),
    ('spread_arb', 'HB_NORTH', 'quantity_mw',  '5.0'),
    ('spread_arb', 'HB_SOUTH', 'window',       '20'),
    ('spread_arb', 'HB_SOUTH', 'threshold',    '1.5'),
    ('spread_arb', 'HB_SOUTH', 'quantity_mw',  '5.0')
ON CONFLICT DO NOTHING;
```

- [ ] **Step 2: Verify the file is valid SQL (no syntax errors)**

```bash
grep -c "spread_arb" /Users/kab/Projects/Mixed_RUST/trade_infra/sql/schema.sql
```

Expected: `7` (6 value rows + 1 comment line).

- [ ] **Step 3: Commit**

```bash
git add sql/schema.sql
git commit -m "feat(spread-arb): seed strategy_configs for HB_NORTH and HB_SOUTH"
git push origin main
```

---

## Task 5: Add `spread-arb` service to `docker-compose.yml`

**Files:**
- Modify: `infra/docker-compose.yml`

- [ ] **Step 1: Open `infra/docker-compose.yml` and add the `spread-arb` service after `ma-crossover-west` (around line 159)**

The existing `ma-crossover-west` service ends at line 159 and `strategy-engine` starts at line 161. Insert between them:

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

- [ ] **Step 2: Validate the YAML is well-formed**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose config --quiet
```

Expected: no output (silent success). If there's a YAML error, the error message shows the line number.

- [ ] **Step 3: Commit**

```bash
git add infra/docker-compose.yml
git commit -m "feat(spread-arb): add spread-arb service to docker-compose"
git push origin main
```

---

## Task 6: Bring up the stack and verify spread-arb signals

**Files:** none — integration verification only.

The postgres container has no named data volume — `docker compose down` destroys the old DB, and `docker compose up` reinitializes from the updated `schema.sql` (including the new `spread_arb` seed rows).

- [ ] **Step 1: Tear down the existing stack**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose down
```

Expected: all containers stop and are removed.

- [ ] **Step 2: Rebuild and start the full stack**

```bash
docker compose up -d --build
```

Expected: all services start (may take ~60s for the `spread-arb` image to build). Watch for errors:

```bash
docker compose ps
```

All services should show `running` or `healthy`. If `spread-arb` shows `exited`, inspect with:
```bash
docker compose logs spread-arb
```

Common failure: `KeyError: 'window'` → the seed INSERT didn't run (postgres data was stale). Confirm `docker compose down` fully removed the postgres container before `up`.

- [ ] **Step 3: Wait for warm-up (spread-arb needs 20 ticks from both nodes before first signal)**

Ticks arrive at 1 Hz. After ~20 seconds, the first signals should appear. Wait 30 seconds then check:

```bash
docker compose logs spread-arb --tail 20
```

Expected output (lines from spread-arb):
```
spread_arb: node_a=HB_NORTH node_b=HB_SOUTH window=20 threshold=1.5
spread_arb: signal ids=3,4 BUY HB_NORTH / SELL HB_SOUTH
```

- [ ] **Step 4: Confirm signals are in the database**

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "SELECT strategy, node, side, quantity_mw, limit_price, created_at
   FROM signals
   WHERE strategy = 'spread_arb'
   ORDER BY created_at DESC
   LIMIT 10;"
```

Expected: rows like:
```
  strategy  |   node   | side | quantity_mw | limit_price |         created_at
------------+----------+------+-------------+-------------+----------------------------
 spread_arb | HB_NORTH | BUY  |        5.00 |     42.3412 | 2026-05-17 ...
 spread_arb | HB_SOUTH | SELL |        5.00 |     39.1023 | 2026-05-17 ...
```

If the table is empty after 60 seconds, check whether the z-score threshold (1.5) is too tight for the current sim volatility. Temporarily lower it to verify the logic fires:
```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "UPDATE strategy_configs SET param_value='0.5'
   WHERE strategy='spread_arb' AND param_key='threshold';"
docker compose restart spread-arb
```

- [ ] **Step 5: Confirm orders were submitted**

The strategy-engine picks up the signals and POSTs to order-svc. Verify:

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "SELECT node, side, quantity_mw, status FROM orders
   WHERE node IN ('HB_NORTH', 'HB_SOUTH')
   ORDER BY created_at DESC
   LIMIT 10;"
```

Expected: `FILLED` orders for HB_NORTH and HB_SOUTH placed by the spread-arb signals.

- [ ] **Step 6: Tag v0.6.0**

```bash
git tag v0.6.0
git push origin v0.6.0
```
