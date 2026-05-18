# Momentum (Rate-of-Change) Strategy Design

**Date:** 2026-05-17
**Version:** v0.14.0

## Goal

Add a momentum strategy based on Rate-of-Change (ROC) — the fourth strategy type alongside mean_reversion, ma_crossover, and spread_arb.

## Context

Existing strategies all fade moves (mean_reversion, spread_arb) or track the trend via EMA crossover (ma_crossover). ROC adds absolute directional momentum: buy when price has risen significantly over the last N ticks, sell when it has fallen. This is opposite in philosophy to mean-reversion.

## Signal Logic

```
ROC = (lmps[-1] - lmps[-window]) / lmps[-window] * 100   (percent)

ROC >  threshold_pct  →  BUY   (upward momentum)
ROC < -threshold_pct  →  SELL  (downward momentum)
otherwise             →  None
```

Guards:
- `len(lmps) < window + 1` → `None` (still warming up; need `lmps[-window]` to exist)
- `lmps[-window] == 0` → `None` (avoid division by zero)

## Architecture

### New files

| File | Purpose |
|------|---------|
| `python/strategies/momentum.py` | Strategy: `compute_momentum_signal` + `run` + argparse |
| `python/tests/test_momentum.py` | 5 unit tests for `compute_momentum_signal` |

### Modified files

| File | Change |
|------|--------|
| `sql/schema.sql` | 9 new `strategy_configs` rows for `momentum` |
| `infra/docker-compose.yml` | 3 new services: `momentum`, `momentum-south`, `momentum-west` |

### No changes to

base.py, strategy-engine, order-svc, risk-svc, Rust code, Prometheus config, Grafana.

## `compute_momentum_signal`

```python
def compute_momentum_signal(
    lmps: list[float],
    window: int,
    threshold_pct: float,
) -> str | None:
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
```

Note: `lmps[-window - 1]` is the value `window` ticks ago; `lmps[-1]` is the current tick. This gives a `window`-tick lookback.

## `run` function

```python
def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'momentum', node)
    window = int(cfg['window'])
    threshold_pct = float(cfg['threshold_pct'])
    quantity_mw = float(cfg['quantity_mw'])

    buf: deque[float] = deque(maxlen=window + 1)

    for tick in listen_ticks(db_url, node):
        buf.append(tick['lmp'])
        side = compute_momentum_signal(list(buf), window, threshold_pct)
        if side:
            signal_id = emit_signal(db_url, 'momentum', node, side, quantity_mw, tick['lmp'])
            print(f"momentum: signal id={signal_id} {side} {quantity_mw}MW @ {tick['lmp']:.4f}")
```

`deque(maxlen=window + 1)` keeps exactly `window + 1` values — enough for the lookback.

## strategy_configs

| strategy | node | param_key | param_value |
|----------|------|-----------|-------------|
| momentum | HB_NORTH | window | 20 |
| momentum | HB_NORTH | threshold_pct | 2.0 |
| momentum | HB_NORTH | quantity_mw | 5.0 |
| momentum | HB_SOUTH | window | 20 |
| momentum | HB_SOUTH | threshold_pct | 2.0 |
| momentum | HB_SOUTH | quantity_mw | 5.0 |
| momentum | HB_WEST | window | 20 |
| momentum | HB_WEST | threshold_pct | 2.0 |
| momentum | HB_WEST | quantity_mw | 4.0 |

`threshold_pct=2.0` means a 2% price move over `window` ticks triggers a signal. With OU σ≈5 and base LMP≈45, a 2% move ($0.90) is ~0.18σ — will fire regularly.

## Docker Compose

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
    ...same... --node HB_SOUTH

  momentum-west:
    ...same... --node HB_WEST
```

## Testing

5 unit tests in `python/tests/test_momentum.py`:

| Test | Input | Expected |
|------|-------|----------|
| `test_below_window_returns_none` | `len < window+1` | `None` |
| `test_buy_signal_on_rise` | 20 ticks at 45.0, last at 46.0 → ROC=2.22% > 2.0 | `'BUY'` |
| `test_sell_signal_on_fall` | 20 ticks at 45.0, last at 43.0 → ROC=-4.44% < -2.0 | `'SELL'` |
| `test_within_band_returns_none` | 20 ticks at 45.0, last at 45.5 → ROC=1.11% < 2.0 | `None` |
| `test_zero_base_returns_none` | `lmps[-window-1] == 0.0` | `None` |

## Scope

- 2 new Python files, 2 modified config files
- No strategy-engine changes (strategy-engine is strategy-agnostic)
- No Rust/Go changes
- Stack grows from 18 to 21 containers
