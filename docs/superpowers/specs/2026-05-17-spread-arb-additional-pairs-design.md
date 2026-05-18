# Additional Spread-Arb Node Pairs Design

**Date:** 2026-05-17
**Version:** v0.12.0

## Goal

Add HB_NORTH/HB_WEST and HB_SOUTH/HB_WEST spread-arb strategy instances alongside the existing HB_NORTH/HB_SOUTH pair. Pure config — no Python logic changes.

## Context

`spread_arb.py` accepts `--node-a` and `--node-b` and loads per-node configs via `load_config(db_url, 'spread_arb', node)`. The existing pair uses HB_NORTH and HB_SOUTH, both of which already have `strategy_configs` rows. HB_WEST needs new config rows; HB_NORTH and HB_SOUTH rows are already present.

## Changes

### `sql/schema.sql`

Add 3 new `strategy_configs` rows for `spread_arb` × HB_WEST, appended to the existing spread_arb INSERT block:

```sql
('spread_arb', 'HB_WEST', 'window',       '20'),
('spread_arb', 'HB_WEST', 'threshold',    '1.5'),
('spread_arb', 'HB_WEST', 'quantity_mw',  '4.0')
```

`quantity_mw=4.0` matches HB_WEST's existing mean_reversion config (smaller position for the lower-volatility node). `window` and `threshold` match the other spread_arb configs.

### `infra/docker-compose.yml`

Two new services appended after the existing `spread-arb` service:

```yaml
  spread-arb-nw:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/spread_arb.py --node-a HB_NORTH --node-b HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      PYTHONUNBUFFERED: "1"
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  spread-arb-sw:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/spread_arb.py --node-a HB_SOUTH --node-b HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      PYTHONUNBUFFERED: "1"
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
```

## Scope

- No Python code changes
- No strategy-engine changes
- No Prometheus config changes (signals/orders already carry `strategy` + `node` labels)
- Total new containers: 2 (spread-arb-nw, spread-arb-sw); stack grows from 16 to 18
- The existing `spread-arb` service (HB_NORTH/HB_SOUTH) is unchanged
