# Momentum Strategy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Rate-of-Change momentum strategy that buys on upward price acceleration and sells on downward, complementing the existing mean-reversion and EMA-crossover strategies.

**Architecture:** `compute_momentum_signal` is a pure function; `run()` uses `deque(maxlen=window+1)` to maintain a rolling buffer and calls `listen_ticks` / `emit_signal` from `strategies.base`. Three Docker Compose services (one per node) reuse the existing `Dockerfile.strategy`. Nine new `strategy_configs` rows seed the parameters.

**Tech Stack:** Python 3.12, `collections.deque`, `strategies.base` (listen_ticks, emit_signal, load_config), uv, PostgreSQL LISTEN/NOTIFY, Docker Compose.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `python/tests/test_momentum.py` | Create | 5 unit tests (failing until Task 2) |
| `python/strategies/momentum.py` | Create | `compute_momentum_signal` + `run` + argparse |
| `sql/schema.sql` | Modify | Append 9 momentum strategy_configs rows |
| `infra/docker-compose.yml` | Modify | Add `momentum`, `momentum-south`, `momentum-west` |

---

## Task 1: Write 5 failing tests

**Files:**
- Create: `python/tests/test_momentum.py`

`momentum.py` doesn't exist yet — imports fail with `ModuleNotFoundError`. That is the expected failing state.

- [ ] **Step 1: Create `python/tests/test_momentum.py`**

```python
import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.momentum import compute_momentum_signal


def test_below_window_returns_none():
    # Only 5 lmps, window=20 — need window+1=21 values
    result = compute_momentum_signal([45.0] * 5, window=20, threshold_pct=2.0)
    assert result is None


def test_buy_signal_on_rise():
    # 21 values: first=45.0, last=46.0 → ROC = (46-45)/45*100 = 2.22% > 2.0 → BUY
    lmps = [45.0] * 20 + [46.0]
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result == 'BUY'


def test_sell_signal_on_fall():
    # 21 values: first=45.0, last=43.0 → ROC = (43-45)/45*100 = -4.44% < -2.0 → SELL
    lmps = [45.0] * 20 + [43.0]
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result == 'SELL'


def test_within_band_returns_none():
    # 21 values: first=45.0, last=45.5 → ROC = (45.5-45)/45*100 = 1.11% < 2.0 → None
    lmps = [45.0] * 20 + [45.5]
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result is None


def test_zero_base_returns_none():
    # base price is 0.0 → division by zero guard → None
    lmps = [0.0] + [45.0] * 20
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result is None
```

- [ ] **Step 2: Run to confirm all 5 fail with `ModuleNotFoundError`**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run pytest tests/test_momentum.py -v
```

Expected: all 5 fail with `ModuleNotFoundError: No module named 'strategies.momentum'`

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/python/tests/test_momentum.py
git commit -m "test(momentum): add 5 failing tests for compute_momentum_signal"
git push origin main
```

---

## Task 2: Implement `momentum.py`

**Files:**
- Create: `python/strategies/momentum.py`

- [ ] **Step 1: Create `python/strategies/momentum.py`**

```python
import argparse
from collections import deque

from strategies.base import emit_signal, listen_ticks, load_config


def compute_momentum_signal(
    lmps: list[float],
    window: int,
    threshold_pct: float,
) -> str | None:
    """Return 'BUY', 'SELL', or None based on Rate-of-Change over window ticks."""
    if len(lmps) < window + 1:
        return None
    base = lmps[-window - 1]
    if base == 0:
        return None
    roc = (lmps[-1] - base) / base * 100
    if roc > threshold_pct:
        return 'BUY'
    if roc < -threshold_pct:
        return 'SELL'
    return None


def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'momentum', node)
    window = int(cfg['window'])
    threshold_pct = float(cfg['threshold_pct'])
    quantity_mw = float(cfg['quantity_mw'])

    buf: deque[float] = deque(maxlen=window + 1)

    print(f"momentum: node={node} window={window} threshold_pct={threshold_pct}")

    for tick in listen_ticks(db_url, node):
        buf.append(tick['lmp'])
        side = compute_momentum_signal(list(buf), window, threshold_pct)
        if side:
            signal_id = emit_signal(db_url, 'momentum', node, side, quantity_mw, tick['lmp'])
            print(f"momentum: signal id={signal_id} {side} {quantity_mw}MW @ {tick['lmp']:.4f}")


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node', default='HB_NORTH')
    p.add_argument('--db-url', default=os.getenv(
        'DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra',
    ))
    args = p.parse_args()
    run(args.db_url, args.node)
```

- [ ] **Step 2: Run the 5 momentum tests — all must pass**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run pytest tests/test_momentum.py -v
```

Expected:
```
tests/test_momentum.py::test_below_window_returns_none PASSED
tests/test_momentum.py::test_buy_signal_on_rise PASSED
tests/test_momentum.py::test_sell_signal_on_fall PASSED
tests/test_momentum.py::test_within_band_returns_none PASSED
tests/test_momentum.py::test_zero_base_returns_none PASSED

5 passed
```

- [ ] **Step 3: Run the full Python test suite — no regressions**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/python
uv run pytest tests/ -v
```

Expected: all tests pass (was 27 before; now 32).

- [ ] **Step 4: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/python/strategies/momentum.py
git commit -m "feat(momentum): implement compute_momentum_signal and run loop"
git push origin main
```

---

## Task 3: Add `strategy_configs` seed rows

**Files:**
- Modify: `sql/schema.sql`

- [ ] **Step 1: Append a new INSERT block at the end of `sql/schema.sql`**

Add after the last existing `ON CONFLICT DO NOTHING;` line:

```sql
-- Momentum strategy config (Rate-of-Change, all 3 nodes)
INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('momentum', 'HB_NORTH', 'window',        '20'),
    ('momentum', 'HB_NORTH', 'threshold_pct', '2.0'),
    ('momentum', 'HB_NORTH', 'quantity_mw',   '5.0'),
    ('momentum', 'HB_SOUTH', 'window',        '20'),
    ('momentum', 'HB_SOUTH', 'threshold_pct', '2.0'),
    ('momentum', 'HB_SOUTH', 'quantity_mw',   '5.0'),
    ('momentum', 'HB_WEST',  'window',        '20'),
    ('momentum', 'HB_WEST',  'threshold_pct', '2.0'),
    ('momentum', 'HB_WEST',  'quantity_mw',   '4.0')
ON CONFLICT DO NOTHING;
```

Note: the config key is `threshold_pct` (not `threshold`) to distinguish it from the `threshold` σ-multiplier used by `mean_reversion` and `spread_arb`.

- [ ] **Step 2: Verify count**

```bash
grep -c "momentum" /Users/kab/Projects/Mixed_RUST/trade_infra/sql/schema.sql
```

Expected: `10` (1 comment line + 9 value rows).

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/sql/schema.sql
git commit -m "feat(momentum): seed strategy_configs for all 3 nodes"
git push origin main
```

---

## Task 4: Add `momentum`, `momentum-south`, `momentum-west` to docker-compose.yml

**Files:**
- Modify: `infra/docker-compose.yml`

Insert the three new services after `spread-arb-sw` (line ~204) and before `strategy-engine` (line ~206).

- [ ] **Step 1: Add the three services to `infra/docker-compose.yml`**

```yaml
  momentum:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/momentum.py --node HB_NORTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  momentum-south:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/momentum.py --node HB_SOUTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  momentum-west:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/momentum.py --node HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
```

- [ ] **Step 2: Validate YAML**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose config --quiet
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/infra/docker-compose.yml
git commit -m "feat(momentum): add momentum, momentum-south, momentum-west Compose services"
git push origin main
```

---

## Task 5: Integration verification and v0.14.0 tag

**Files:** none — integration only.

- [ ] **Step 1: Tear down the existing stack**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose down
```

- [ ] **Step 2: Rebuild and start**

```bash
docker compose up -d --build
```

Python strategy images are cached — only `momentum`, `momentum-south`, `momentum-west` build fresh. Wait ~30s.

- [ ] **Step 3: Confirm 3 new services are running**

```bash
docker compose ps | grep momentum
```

Expected: three running containers:
```
infra-momentum-1        running
infra-momentum-south-1  running
infra-momentum-west-1   running
```

- [ ] **Step 4: Check startup logs**

```bash
docker compose logs momentum --tail 5
```

Expected:
```
momentum: node=HB_NORTH window=20 threshold_pct=2.0
```

If you see `KeyError: 'threshold_pct'` → the seed rows didn't load. Ensure `docker compose down` fully removed the postgres container before `up`.

- [ ] **Step 5: Wait ~30s then verify signals**

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "SELECT strategy, node, side, quantity_mw, limit_price FROM signals
   WHERE strategy='momentum'
   ORDER BY created_at DESC LIMIT 9;"
```

Expected: rows for HB_NORTH, HB_SOUTH, HB_WEST with `quantity_mw=5.00` or `4.00`.

If no signals after 60s, the OU price moves may not be exceeding the 2% threshold. Lower it temporarily:
```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "UPDATE strategy_configs SET param_value='0.5'
   WHERE strategy='momentum' AND param_key='threshold_pct';"
docker compose restart momentum momentum-south momentum-west
```

- [ ] **Step 6: Tag v0.14.0**

```bash
cd /Users/kab/Projects/Mixed_RUST
git tag v0.14.0
git push origin v0.14.0
```
