# Additional Spread-Arb Node Pairs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add HB_NORTH/HB_WEST and HB_SOUTH/HB_WEST spread-arb instances alongside the existing HB_NORTH/HB_SOUTH pair.

**Architecture:** Two config-only changes — 3 new `strategy_configs` rows for HB_WEST in `sql/schema.sql`, and 2 new Docker Compose services (`spread-arb-nw`, `spread-arb-sw`) that reuse the existing `spread_arb.py` with different `--node-a`/`--node-b` arguments. No Python logic changes.

**Tech Stack:** PostgreSQL seed SQL, Docker Compose, existing `spread_arb.py`.

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `sql/schema.sql` | Modify | Add 3 HB_WEST rows to spread_arb INSERT block |
| `infra/docker-compose.yml` | Modify | Add `spread-arb-nw` and `spread-arb-sw` services |

---

## Task 1: Add HB_WEST config rows and new Compose services

**Files:**
- Modify: `sql/schema.sql` (after line 101)
- Modify: `infra/docker-compose.yml` (after line 178)

**Important:** The git root is `/Users/kab/Projects/Mixed_RUST/`. All git commands must run from there.

- [ ] **Step 1: Append HB_WEST rows to the spread_arb INSERT block in `sql/schema.sql`**

The current last block of `sql/schema.sql` (lines 93–101) is:
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

Replace it with:
```sql
-- Spread-arb strategy config (HB_NORTH / HB_SOUTH / HB_WEST)
INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('spread_arb', 'HB_NORTH', 'window',       '20'),
    ('spread_arb', 'HB_NORTH', 'threshold',    '1.5'),
    ('spread_arb', 'HB_NORTH', 'quantity_mw',  '5.0'),
    ('spread_arb', 'HB_SOUTH', 'window',       '20'),
    ('spread_arb', 'HB_SOUTH', 'threshold',    '1.5'),
    ('spread_arb', 'HB_SOUTH', 'quantity_mw',  '5.0'),
    ('spread_arb', 'HB_WEST',  'window',       '20'),
    ('spread_arb', 'HB_WEST',  'threshold',    '1.5'),
    ('spread_arb', 'HB_WEST',  'quantity_mw',  '4.0')
ON CONFLICT DO NOTHING;
```

`quantity_mw=4.0` for HB_WEST matches the lower-volatility node's existing mean_reversion config.

- [ ] **Step 2: Verify the grep count**

```bash
grep -c "spread_arb" /Users/kab/Projects/Mixed_RUST/trade_infra/sql/schema.sql
```

Expected: `10` (1 comment + 9 value rows).

- [ ] **Step 3: Add `spread-arb-nw` and `spread-arb-sw` services to `infra/docker-compose.yml`**

Find the `spread-arb` service block (lines 167–178) which ends with `restart: unless-stopped`. Insert the two new services immediately after it (before `strategy-engine`):

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

- [ ] **Step 4: Validate YAML**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose config --quiet
```

Expected: no output (silent success).

- [ ] **Step 5: Commit and push from git root**

```bash
cd /Users/kab/Projects/Mixed_RUST
git add trade_infra/sql/schema.sql trade_infra/infra/docker-compose.yml
git commit -m "feat(spread-arb): add HB_NORTH/HB_WEST and HB_SOUTH/HB_WEST pairs (v0.12.0)"
git push origin main
```

---

## Task 2: Integration verification and v0.12.0 tag

**Files:** none — integration verification only.

The postgres container has no named data volume — `docker compose down` destroys the old DB. `docker compose up` reinitializes from the updated `schema.sql` (including the new HB_WEST rows).

- [ ] **Step 1: Tear down the existing stack**

```bash
cd /Users/kab/Projects/Mixed_RUST/trade_infra/infra
docker compose down
```

Expected: all containers stop and are removed.

- [ ] **Step 2: Rebuild and start**

```bash
docker compose up -d --build
```

The Python strategy images are cached — only the spread-arb-nw and spread-arb-sw services are added, not rebuilt from scratch. Wait ~30s for startup.

- [ ] **Step 3: Confirm both new services are running**

```bash
docker compose ps | grep spread-arb
```

Expected: three running spread-arb services:
```
infra-spread-arb-1     running
infra-spread-arb-nw-1  running
infra-spread-arb-sw-1  running
```

- [ ] **Step 4: Check logs for warm-up messages**

```bash
docker compose logs spread-arb-nw --tail 5
docker compose logs spread-arb-sw --tail 5
```

Expected (each service):
```
spread_arb: node_a=HB_NORTH node_b=HB_WEST window=20 threshold=1.5
spread_arb: node_a=HB_SOUTH node_b=HB_WEST window=20 threshold=1.5
```

If a service shows `KeyError: 'window'` — the HB_WEST seed rows didn't load. Confirm `docker compose down` fully removed the postgres container before `up`.

- [ ] **Step 5: Verify signals appear after warm-up (~30s)**

```bash
docker compose exec postgres psql -U postgres -d trade_infra -c \
  "SELECT strategy, node, side, quantity_mw FROM signals
   WHERE strategy='spread_arb'
   ORDER BY created_at DESC LIMIT 12;"
```

Expected: rows for all three pairs — `HB_NORTH`/`HB_SOUTH`, `HB_NORTH`/`HB_WEST`, `HB_SOUTH`/`HB_WEST` — appearing in pairs (BUY + SELL legs per signal event).

- [ ] **Step 6: Tag v0.12.0**

```bash
cd /Users/kab/Projects/Mixed_RUST
git tag v0.12.0
git push origin v0.12.0
```
