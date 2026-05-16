# Multi-Node Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend trade_infra from one ERCOT node (HB_NORTH) to three (HB_NORTH, HB_SOUTH, HB_WEST) with independent market data, strategies, and per-node position tracking.

**Architecture:** No logic changes to any service binary except making risk-svc's position limit configurable via env var. All other changes are pure config/data: 12 new strategy_configs seed rows, 6 new Docker Compose service blocks, 2 new Prometheus scrape targets.

**Tech Stack:** PostgreSQL (seed data), Go (one env-var read in risk-svc), Docker Compose, Prometheus.

---

## File Map

```
sql/schema.sql                              — append 12 strategy_configs rows
risk-svc/internal/listener/listener.go      — const → env-var read
infra/docker-compose.yml                    — add 6 services + RISK_POSITION_LIMIT_MW to risk-svc
infra/prometheus.yml                        — add 2 scrape targets
```

---

### Task 1: Seed strategy_configs for HB_SOUTH and HB_WEST

**Files:**
- Modify: `sql/schema.sql`

- [ ] **Step 1: Append seed rows to sql/schema.sql**

Add the following block to the **end** of `sql/schema.sql`, after the existing HB_NORTH INSERT:

```sql
-- HB_SOUTH strategy config
INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('mean_reversion', 'HB_SOUTH', 'window',       '20'),
    ('mean_reversion', 'HB_SOUTH', 'threshold',    '1.0'),
    ('mean_reversion', 'HB_SOUTH', 'quantity_mw',  '5.0'),
    ('ma_crossover',   'HB_SOUTH', 'fast_period',  '5'),
    ('ma_crossover',   'HB_SOUTH', 'slow_period',  '20'),
    ('ma_crossover',   'HB_SOUTH', 'quantity_mw',  '5.0'),
-- HB_WEST strategy config (higher threshold + smaller qty — lower volatility node)
    ('mean_reversion', 'HB_WEST',  'window',       '20'),
    ('mean_reversion', 'HB_WEST',  'threshold',    '1.2'),
    ('mean_reversion', 'HB_WEST',  'quantity_mw',  '4.0'),
    ('ma_crossover',   'HB_WEST',  'fast_period',  '5'),
    ('ma_crossover',   'HB_WEST',  'slow_period',  '20'),
    ('ma_crossover',   'HB_WEST',  'quantity_mw',  '4.0')
ON CONFLICT DO NOTHING;
```

- [ ] **Step 2: Apply to local databases**

```bash
psql trade_infra -f sql/schema.sql
psql trade_infra_test -f sql/schema.sql
```

Expected output includes `INSERT 0 12` (or `INSERT 0 0` if rows already exist — idempotent).

- [ ] **Step 3: Verify 18 total rows**

```bash
psql trade_infra -c "SELECT node, COUNT(*) FROM strategy_configs GROUP BY node ORDER BY node"
```

Expected:
```
   node   | count
----------+-------
 HB_NORTH |     6
 HB_SOUTH |     6
 HB_WEST  |     6
(3 rows)
```

- [ ] **Step 4: Commit and push**

```bash
git add sql/schema.sql
git commit -m "feat(sql): seed strategy_configs for HB_SOUTH and HB_WEST"
git push origin main
```

---

### Task 2: Make risk-svc position limit configurable via env var

**Files:**
- Modify: `risk-svc/internal/listener/listener.go`

- [ ] **Step 1: Replace the hardcoded constant**

In `risk-svc/internal/listener/listener.go`, replace:

```go
const positionLimitMW = 50.0
```

With:

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

- [ ] **Step 2: Add the missing imports**

The import block currently starts with:

```go
import (
	"database/sql"
	"encoding/json"
	"log"
	"time"
```

Add `"os"` and `"strconv"` so it reads:

```go
import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"
```

- [ ] **Step 3: Build to verify it compiles**

```bash
cd risk-svc && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 4: Commit and push**

```bash
git add risk-svc/internal/listener/listener.go
git commit -m "feat(risk-svc): make positionLimitMW configurable via RISK_POSITION_LIMIT_MW env var"
git push origin main
```

---

### Task 3: Add 6 service blocks and RISK_POSITION_LIMIT_MW to docker-compose.yml

**Files:**
- Modify: `infra/docker-compose.yml`

- [ ] **Step 1: Add RISK_POSITION_LIMIT_MW to the existing risk-svc environment block**

In the `risk-svc` service, the `environment:` block currently ends at `METRICS_ADDR: ":9103"`. Add one line so it reads:

```yaml
  risk-svc:
    build:
      context: ../risk-svc
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      API_ADDR: ":8081"
      METRICS_ADDR: ":9103"
      RISK_POSITION_LIMIT_MW: "50"
    ports: ["8081:8081", "9103:9103"]
    depends_on:
      postgres:
        condition: service_healthy
```

- [ ] **Step 2: Add the 6 new service blocks**

Insert the following YAML **after the `market-data-svc` block and before the `order-svc` block**:

```yaml
  market-data-svc-south:
    build:
      context: ../market-data-svc
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_SOUTH
      BASE_LMP: "42.0"
      VOLATILITY: "7.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
    depends_on:
      postgres:
        condition: service_healthy

  market-data-svc-west:
    build:
      context: ../market-data-svc
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      NODE_NAME: HB_WEST
      BASE_LMP: "38.0"
      VOLATILITY: "6.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
    depends_on:
      postgres:
        condition: service_healthy

  mean-reversion-south:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/mean_reversion.py --node HB_SOUTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  mean-reversion-west:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/mean_reversion.py --node HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  ma-crossover-south:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/ma_crossover.py --node HB_SOUTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  ma-crossover-west:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/ma_crossover.py --node HB_WEST
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
```

Note: `market-data-svc-south` and `market-data-svc-west` have no host port mapping — they use METRICS_PORT=9101 internally but are only scraped by Prometheus within the Docker network via service name. No conflict with the original `market-data-svc` which exposes 9101 externally.

- [ ] **Step 3: Verify YAML is valid**

```bash
docker compose -f infra/docker-compose.yml config --quiet && echo "YAML valid"
```

Expected: `YAML valid`

- [ ] **Step 4: Commit and push**

```bash
git add infra/docker-compose.yml
git commit -m "feat(infra): add HB_SOUTH and HB_WEST market-data and strategy services"
git push origin main
```

---

### Task 4: Add Prometheus scrape targets for new market-data-svc instances

**Files:**
- Modify: `infra/prometheus.yml`

- [ ] **Step 1: Append scrape configs**

Add the following two jobs to the end of the `scrape_configs` list in `infra/prometheus.yml`:

```yaml
  - job_name: market-data-svc-south
    static_configs:
      - targets: ["market-data-svc-south:9101"]

  - job_name: market-data-svc-west
    static_configs:
      - targets: ["market-data-svc-west:9101"]
```

- [ ] **Step 2: Commit and push**

```bash
git add infra/prometheus.yml
git commit -m "feat(infra): add Prometheus scrape targets for HB_SOUTH and HB_WEST"
git push origin main
```

---

### Task 5: Bring up full stack and verify all 3 nodes

**Files:** none — verification only

- [ ] **Step 1: Rebuild and start**

```bash
cd infra && docker compose down -v && docker compose up --build -d
```

Wait for all containers to reach healthy/running state:

```bash
docker compose ps
```

Expected: 15 containers running (postgres, market-data-svc ×3, order-svc, risk-svc, mean-reversion ×3, ma-crossover ×3, strategy-engine, prometheus, grafana).

- [ ] **Step 2: Wait for warm-up then verify ticks on all 3 nodes**

```bash
until docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT COUNT(DISTINCT node) FROM price_ticks" \
  2>/dev/null | grep -q " 3"; do sleep 3; done && echo "All 3 nodes producing ticks"
```

Then check counts:

```bash
docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT node, COUNT(*) AS ticks FROM price_ticks GROUP BY node ORDER BY node"
```

Expected:
```
   node   | ticks
----------+-------
 HB_NORTH |   25+
 HB_SOUTH |   25+
 HB_WEST  |   25+
```

- [ ] **Step 3: Wait for signals from all 3 nodes**

```bash
until docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT COUNT(DISTINCT node) FROM signals" \
  2>/dev/null | grep -q " 3"; do sleep 5; done && echo "All 3 nodes generating signals"
```

Then check submitted signals per node:

```bash
docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT node, strategy, status, COUNT(*) FROM signals GROUP BY node, strategy, status ORDER BY node, strategy"
```

Expected: rows for HB_NORTH, HB_SOUTH, HB_WEST with PENDING/SUBMITTED/SKIPPED signals.

- [ ] **Step 4: Verify per-node position tracking**

```bash
docker compose exec -T postgres psql -U postgres trade_infra \
  -c "SELECT node, net_mw, avg_price FROM positions ORDER BY node"
```

Expected: up to 3 rows (one per node that has had at least one fill), each with independent `net_mw` and `avg_price`.

- [ ] **Step 5: Verify strategy-engine logs show all 3 nodes**

```bash
docker compose logs strategy-engine 2>&1 | grep -E "submitted|skipped" | tail -15
```

Expected: log lines referencing `HB_NORTH`, `HB_SOUTH`, and `HB_WEST`.

- [ ] **Step 6: Commit tag**

```bash
git tag -a v0.3.0 -m "v0.3.0 — multi-node support: HB_SOUTH + HB_WEST"
git push origin v0.3.0
```

---

## Spec Coverage Check

| Spec requirement | Task |
|---|---|
| 12 seed rows for HB_SOUTH and HB_WEST in strategy_configs | Task 1 |
| HB_WEST threshold=1.2, quantity_mw=4.0 (per-node tuning) | Task 1 |
| positionLimitMW reads RISK_POSITION_LIMIT_MW env var, default 50.0 | Task 2 |
| market-data-svc-south (BASE_LMP=42, VOLATILITY=7) | Task 3 |
| market-data-svc-west (BASE_LMP=38, VOLATILITY=6) | Task 3 |
| mean-reversion-south/west, ma-crossover-south/west | Task 3 |
| RISK_POSITION_LIMIT_MW added to risk-svc env block | Task 3 |
| No external port mapping on new market-data-svc instances | Task 3 |
| Prometheus scrape targets for south and west | Task 4 |
| All 3 nodes producing ticks, signals, and positions | Task 5 |
