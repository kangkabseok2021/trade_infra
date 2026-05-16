# Strategy Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two automated trading strategies (mean reversion + MA crossover) as Python daemons that write signals to PostgreSQL, consumed by a new Go strategy-engine service that gates on risk and submits orders to order-svc.

**Architecture:** Python daemons LISTEN on `price_ticks`, compute signals, and INSERT into a `signals` table with `NOTIFY 'signals'`. A Go `strategy-engine` service LISTEN on `signals`, applies position-limit and cooldown checks, then POSTs to `order-svc`. All components share the existing PostgreSQL database.

**Tech Stack:** Python 3.12 + uv + psycopg2 + pytest, Go 1.26 + lib/pq + prometheus/client_golang, PostgreSQL 16, Docker Compose.

---

## File Map

```
sql/schema.sql                               — append signals + strategy_configs + seed rows
python/
├── strategies/
│   ├── __init__.py                          — empty package marker
│   ├── base.py                              — StrategyBase: LISTEN, emit_signal, load_config
│   ├── mean_reversion.py                    — MeanReversion daemon + compute_signal()
│   └── ma_crossover.py                      — MACrossover daemon + compute_ema() + detect_cross()
├── tests/
│   ├── test_mean_reversion.py
│   └── test_ma_crossover.py
└── Dockerfile.strategy                      — new: Python strategy image
strategy-engine/
├── go.mod
├── Dockerfile
├── cmd/server/main.go
├── internal/
│   ├── signal/
│   │   ├── model.go                         — Signal struct, Status constants
│   │   ├── store.go                         — ClaimPending, SetOrderID, LatestSubmitted, GetByID
│   │   └── store_test.go
│   ├── riskstore/
│   │   └── riskstore.go                     — LatestNetExposure (reads risk_snapshots)
│   ├── gate/
│   │   ├── gate.go                          — Check(strategy, node, qty) → allowed, reason
│   │   └── gate_test.go
│   ├── submitter/
│   │   ├── submitter.go                     — POST /orders to order-svc
│   │   └── submitter_test.go
│   └── listener/
│       └── listener.go                      — LISTEN 'signals', orchestrates gate + submitter
└── metrics/metrics.go
infra/docker-compose.yml                     — add mean-reversion, ma-crossover, strategy-engine
infra/prometheus.yml                         — add strategy-engine scrape target
python/smoke_test.py                         — extend with strategy signal verification
```

---

## Phase 1: SQL Schema Additions

### Task 1: Add signals + strategy_configs tables and seed data

**Files:**
- Modify: `sql/schema.sql`

- [ ] **Step 1: Append to sql/schema.sql**

Add the following to the end of `sql/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS signals (
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
CREATE INDEX IF NOT EXISTS idx_signals_strategy_node_ts
    ON signals (strategy, node, created_at DESC);

CREATE TABLE IF NOT EXISTS strategy_configs (
    strategy    TEXT NOT NULL,
    node        TEXT NOT NULL,
    param_key   TEXT NOT NULL,
    param_value TEXT NOT NULL,
    PRIMARY KEY (strategy, node, param_key)
);

INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('mean_reversion', 'HB_NORTH', 'window',       '20'),
    ('mean_reversion', 'HB_NORTH', 'threshold',    '1.0'),
    ('mean_reversion', 'HB_NORTH', 'quantity_mw',  '5.0'),
    ('ma_crossover',   'HB_NORTH', 'fast_period',  '5'),
    ('ma_crossover',   'HB_NORTH', 'slow_period',  '20'),
    ('ma_crossover',   'HB_NORTH', 'quantity_mw',  '5.0')
ON CONFLICT DO NOTHING;
```

- [ ] **Step 2: Apply to both local databases**

```bash
psql trade_infra -f sql/schema.sql
psql trade_infra_test -f sql/schema.sql
```

Expected:
```
CREATE TABLE
CREATE INDEX
CREATE TABLE
INSERT 0 6
```

- [ ] **Step 3: Verify tables and seed data**

```bash
psql trade_infra -c "\dt signals strategy_configs"
psql trade_infra -c "SELECT * FROM strategy_configs ORDER BY strategy, param_key"
```

Expected: 2 tables listed; 6 config rows for mean_reversion and ma_crossover.

- [ ] **Step 4: Commit**

```bash
git add sql/schema.sql
git commit -m "feat(sql): signals and strategy_configs tables with default config seed"
```

---

## Phase 2: Python Strategies

### Task 2: Python directory scaffold + base.py

**Files:**
- Create: `python/strategies/__init__.py`
- Create: `python/strategies/base.py`

- [ ] **Step 1: Create scaffold**

```bash
mkdir -p python/strategies
touch python/strategies/__init__.py
```

- [ ] **Step 2: Write base.py**

Create `python/strategies/base.py`:

```python
import json
import select
import psycopg2
import psycopg2.extensions


def load_config(db_url: str, strategy: str, node: str) -> dict[str, str]:
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()
    cur.execute(
        "SELECT param_key, param_value FROM strategy_configs WHERE strategy=%s AND node=%s",
        (strategy, node),
    )
    cfg = {row[0]: row[1] for row in cur.fetchall()}
    conn.close()
    return cfg


def listen_ticks(db_url: str, node: str):
    """Yield tick dicts {node, lmp} for the given node via LISTEN price_ticks."""
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
            if payload.get("node") == node:
                yield payload


def emit_signal(
    db_url: str,
    strategy: str,
    node: str,
    side: str,
    quantity_mw: float,
    limit_price: float,
) -> int:
    """Insert a signal row and NOTIFY 'signals'. Returns the signal id."""
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(
        """
        INSERT INTO signals (strategy, node, side, quantity_mw, limit_price)
        VALUES (%s, %s, %s, %s, %s)
        RETURNING id
        """,
        (strategy, node, side, round(quantity_mw, 2), round(limit_price, 4)),
    )
    signal_id = cur.fetchone()[0]
    cur.execute("SELECT pg_notify('signals', %s)", (str(signal_id),))
    conn.close()
    return signal_id
```

- [ ] **Step 3: Commit**

```bash
git add python/strategies/__init__.py python/strategies/base.py
git commit -m "feat(python): strategy base helpers — listen_ticks, emit_signal, load_config"
```

---

### Task 3: mean_reversion.py (TDD)

**Files:**
- Create: `python/tests/test_mean_reversion.py`
- Create: `python/strategies/mean_reversion.py`

- [ ] **Step 1: Write failing tests**

Create `python/tests/test_mean_reversion.py`:

```python
import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.mean_reversion import compute_signal


def test_no_signal_during_warmup():
    # Fewer ticks than window → still warming up
    result = compute_signal([45.0] * 5, window=20, threshold=1.0)
    assert result is None


def test_buy_signal_below_band():
    # 19 ticks at 45.0, last tick at 30.0 — far below mean
    # mean≈44.25, std≈3.36 → lower band≈40.9; 30.0 < 40.9 → BUY
    lmps = [45.0] * 19 + [30.0]
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result == 'BUY'


def test_sell_signal_above_band():
    # 19 ticks at 45.0, last tick at 60.0 — far above mean
    # mean≈45.75, std≈3.35 → upper band≈49.1; 60.0 > 49.1 → SELL
    lmps = [45.0] * 19 + [60.0]
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result == 'SELL'


def test_no_signal_within_band():
    # Alternating 44/46: mean=45, std≈1.03 → band=[43.97, 46.03]; last=46.0 inside
    lmps = [44.0, 46.0] * 10
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result is None


def test_no_signal_flat_prices():
    # All same → std=0 → no signal (avoids spurious fires)
    result = compute_signal([45.0] * 20, window=20, threshold=1.0)
    assert result is None


def test_only_last_window_used():
    # First 20 values are noise; last 20 are flat at 45.0 → no signal
    lmps = [1.0] * 20 + [45.0] * 20
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result is None
```

- [ ] **Step 2: Run to verify failure**

```bash
cd python && uv run pytest tests/test_mean_reversion.py -v 2>&1 | head -8
```

Expected: `ImportError: cannot import name 'compute_signal'`

- [ ] **Step 3: Implement mean_reversion.py**

Create `python/strategies/mean_reversion.py`:

```python
import argparse
import statistics
from collections import deque

from strategies.base import emit_signal, listen_ticks, load_config


def compute_signal(lmps: list[float], window: int, threshold: float) -> str | None:
    """Return 'BUY', 'SELL', or None based on mean-reversion logic."""
    if len(lmps) < window:
        return None
    window_data = lmps[-window:]
    if len(window_data) < 2:
        return None
    mean = statistics.mean(window_data)
    std = statistics.stdev(window_data)
    if std == 0:
        return None
    current = window_data[-1]
    if current < mean - threshold * std:
        return 'BUY'
    if current > mean + threshold * std:
        return 'SELL'
    return None


def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'mean_reversion', node)
    window = int(cfg['window'])
    threshold = float(cfg['threshold'])
    quantity_mw = float(cfg['quantity_mw'])

    buf: deque[float] = deque(maxlen=window)
    print(f"mean_reversion: node={node} window={window} threshold={threshold}")

    for tick in listen_ticks(db_url, node):
        buf.append(tick['lmp'])
        side = compute_signal(list(buf), window, threshold)
        if side:
            limit_price = tick['lmp']
            signal_id = emit_signal(db_url, 'mean_reversion', node, side, quantity_mw, limit_price)
            print(f"mean_reversion: signal id={signal_id} {side} {quantity_mw}MW @ {limit_price:.4f}")


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node', default='HB_NORTH')
    p.add_argument('--db-url', default=os.getenv('DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra'))
    args = p.parse_args()
    run(args.db_url, args.node)
```

- [ ] **Step 4: Run tests — expect all pass**

```bash
cd python && uv run pytest tests/test_mean_reversion.py -v
```

Expected:
```
PASSED tests/test_mean_reversion.py::test_no_signal_during_warmup
PASSED tests/test_mean_reversion.py::test_buy_signal_below_band
PASSED tests/test_mean_reversion.py::test_sell_signal_above_band
PASSED tests/test_mean_reversion.py::test_no_signal_within_band
PASSED tests/test_mean_reversion.py::test_no_signal_flat_prices
PASSED tests/test_mean_reversion.py::test_only_last_window_used
6 passed
```

- [ ] **Step 5: Commit**

```bash
git add python/strategies/mean_reversion.py python/tests/test_mean_reversion.py
git commit -m "feat(python): mean_reversion strategy with compute_signal (TDD)"
```

---

### Task 4: ma_crossover.py (TDD)

**Files:**
- Create: `python/tests/test_ma_crossover.py`
- Create: `python/strategies/ma_crossover.py`

- [ ] **Step 1: Write failing tests**

Create `python/tests/test_ma_crossover.py`:

```python
import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.ma_crossover import compute_ema, detect_cross


def test_ema_moves_toward_new_value():
    # alpha = 2/(5+1) ≈ 0.333; ema = 0.333*60 + 0.667*40 ≈ 46.67
    result = compute_ema(lmp=60.0, prev_ema=40.0, period=5)
    assert abs(result - 46.667) < 0.01


def test_ema_unchanged_at_same_value():
    result = compute_ema(lmp=45.0, prev_ema=45.0, period=5)
    assert result == 45.0


def test_buy_cross_fast_above_slow():
    # fast was below slow, now above → BUY
    result = detect_cross(fast_prev=44.0, fast_curr=46.0, slow_prev=45.0, slow_curr=45.0)
    assert result == 'BUY'


def test_sell_cross_fast_below_slow():
    # fast was above slow, now below → SELL
    result = detect_cross(fast_prev=46.0, fast_curr=44.0, slow_prev=45.0, slow_curr=45.0)
    assert result == 'SELL'


def test_no_cross_fast_stays_above():
    result = detect_cross(fast_prev=47.0, fast_curr=46.0, slow_prev=45.0, slow_curr=45.5)
    assert result is None


def test_no_cross_fast_stays_below():
    result = detect_cross(fast_prev=43.0, fast_curr=44.0, slow_prev=45.0, slow_curr=45.0)
    assert result is None


def test_no_cross_equal_values():
    # fast == slow on both sides → no cross
    result = detect_cross(fast_prev=45.0, fast_curr=45.0, slow_prev=45.0, slow_curr=45.0)
    assert result is None
```

- [ ] **Step 2: Run to verify failure**

```bash
cd python && uv run pytest tests/test_ma_crossover.py -v 2>&1 | head -5
```

Expected: `ImportError: cannot import name 'compute_ema'`

- [ ] **Step 3: Implement ma_crossover.py**

Create `python/strategies/ma_crossover.py`:

```python
import argparse

from strategies.base import emit_signal, listen_ticks, load_config


def compute_ema(lmp: float, prev_ema: float, period: int) -> float:
    """Exponential moving average: α*lmp + (1-α)*prev_ema, α = 2/(period+1)."""
    alpha = 2.0 / (period + 1)
    return alpha * lmp + (1.0 - alpha) * prev_ema


def detect_cross(
    fast_prev: float, fast_curr: float,
    slow_prev: float, slow_curr: float,
) -> str | None:
    """Return 'BUY' on fast-crosses-above-slow, 'SELL' on fast-crosses-below, else None."""
    prev_above = fast_prev > slow_prev
    curr_above = fast_curr > slow_curr
    if not prev_above and curr_above:
        return 'BUY'
    if prev_above and not curr_above:
        return 'SELL'
    return None


def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'ma_crossover', node)
    fast_period = int(cfg['fast_period'])
    slow_period = int(cfg['slow_period'])
    quantity_mw = float(cfg['quantity_mw'])

    fast_ema: float | None = None
    slow_ema: float | None = None
    prev_fast: float | None = None
    prev_slow: float | None = None
    tick_count = 0

    print(f"ma_crossover: node={node} fast={fast_period} slow={slow_period}")

    for tick in listen_ticks(db_url, node):
        lmp = tick['lmp']
        tick_count += 1

        if fast_ema is None:
            fast_ema = slow_ema = lmp
            prev_fast = prev_slow = lmp
            continue

        new_fast = compute_ema(lmp, fast_ema, fast_period)
        new_slow = compute_ema(lmp, slow_ema, slow_period)

        if tick_count > slow_period:
            side = detect_cross(fast_ema, new_fast, slow_ema, new_slow)
            if side:
                signal_id = emit_signal(db_url, 'ma_crossover', node, side, quantity_mw, lmp)
                print(f"ma_crossover: signal id={signal_id} {side} {quantity_mw}MW @ {lmp:.4f}")

        fast_ema = new_fast
        slow_ema = new_slow


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node', default='HB_NORTH')
    p.add_argument('--db-url', default=os.getenv('DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra'))
    args = p.parse_args()
    run(args.db_url, args.node)
```

- [ ] **Step 4: Run tests**

```bash
cd python && uv run pytest tests/test_ma_crossover.py -v
```

Expected:
```
PASSED tests/test_ma_crossover.py::test_ema_moves_toward_new_value
PASSED tests/test_ma_crossover.py::test_ema_unchanged_at_same_value
PASSED tests/test_ma_crossover.py::test_buy_cross_fast_above_slow
PASSED tests/test_ma_crossover.py::test_sell_cross_fast_below_slow
PASSED tests/test_ma_crossover.py::test_no_cross_fast_stays_above
PASSED tests/test_ma_crossover.py::test_no_cross_fast_stays_below
PASSED tests/test_ma_crossover.py::test_no_cross_equal_values
7 passed
```

- [ ] **Step 5: Run full Python suite**

```bash
cd python && uv run pytest tests/ -v 2>&1 | tail -5
```

Expected: `22 passed` (6 mean_rev + 7 ma_cross + 9 existing).

- [ ] **Step 6: Commit**

```bash
git add python/strategies/ma_crossover.py python/tests/test_ma_crossover.py
git commit -m "feat(python): ma_crossover strategy with compute_ema + detect_cross (TDD)"
```

---

## Phase 3: strategy-engine (Go)

### Task 5: Go module + signal model + store (TDD)

**Files:**
- Create: `strategy-engine/go.mod`
- Create: `strategy-engine/internal/signal/model.go`
- Create: `strategy-engine/internal/signal/store.go`
- Create: `strategy-engine/internal/signal/store_test.go`

- [ ] **Step 1: Scaffold directories and init module**

```bash
mkdir -p strategy-engine/{cmd/server,internal/{signal,riskstore,gate,submitter,listener},metrics}
cd strategy-engine
go mod init github.com/kangkabseok2021/trade_infra/strategy-engine
go get github.com/lib/pq@v1.10.9
go get github.com/prometheus/client_golang@v1.19.0
go mod tidy
```

- [ ] **Step 2: Write model.go**

Create `strategy-engine/internal/signal/model.go`:

```go
package signal

import "time"

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusSubmitted Status = "SUBMITTED"
	StatusSkipped   Status = "SKIPPED"
)

type Signal struct {
	ID         int64     `json:"id"`
	Strategy   string    `json:"strategy"`
	Node       string    `json:"node"`
	Side       string    `json:"side"`
	QuantityMW float64   `json:"quantity_mw"`
	LimitPrice float64   `json:"limit_price"`
	Status     Status    `json:"status"`
	Reason     *string   `json:"reason,omitempty"`
	OrderID    *int64    `json:"order_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
```

- [ ] **Step 3: Write failing store tests**

Create `strategy-engine/internal/signal/store_test.go`:

```go
package signal_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test"
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM signals"); db.Close() })
	return db
}

func insertPending(t *testing.T, s *signal.Store) *signal.Signal {
	t.Helper()
	sig, err := s.Insert("mean_reversion", "HB_NORTH", "BUY", 5.0, 45.0)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return sig
}

func TestStore_Insert(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig, err := s.Insert("mean_reversion", "HB_NORTH", "BUY", 5.0, 45.0)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if sig.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if sig.Status != signal.StatusPending {
		t.Errorf("want PENDING got %s", sig.Status)
	}
}

func TestStore_ClaimPending_Success(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	claimed, err := s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Error("expected claim to succeed")
	}
	got, _ := s.GetByID(sig.ID)
	if got.Status != signal.StatusSubmitted {
		t.Errorf("want SUBMITTED got %s", got.Status)
	}
}

func TestStore_ClaimPending_Idempotent(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	// second claim should return false (already claimed)
	claimed, err := s.ClaimPending(sig.ID, signal.StatusSkipped, nil)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Error("second claim should return false")
	}
}

func TestStore_SetOrderID(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	if err := s.SetOrderID(sig.ID, 42); err != nil {
		t.Fatalf("set order id: %v", err)
	}
	got, _ := s.GetByID(sig.ID)
	if got.OrderID == nil || *got.OrderID != 42 {
		t.Errorf("want order_id=42 got %v", got.OrderID)
	}
}

func TestStore_LatestSubmitted_None(t *testing.T) {
	s := signal.NewStore(testDB(t))
	ts, err := s.LatestSubmitted("mean_reversion", "HB_NORTH")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ts != nil {
		t.Error("expected nil when no submitted signals")
	}
}

func TestStore_LatestSubmitted_Returns(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	ts, err := s.LatestSubmitted("mean_reversion", "HB_NORTH")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ts == nil {
		t.Error("expected non-nil timestamp after submission")
	}
	if time.Since(*ts) > 5*time.Second {
		t.Error("timestamp should be recent")
	}
}
```

- [ ] **Step 4: Run to verify failure**

```bash
cd strategy-engine && go test ./internal/signal/... 2>&1 | head -5
```

Expected: compile error — `store.go` missing.

- [ ] **Step 5: Implement store.go**

Create `strategy-engine/internal/signal/store.go`:

```go
package signal

import (
	"database/sql"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Insert(strategy, node, side string, qtyMW, limitPrice float64) (*Signal, error) {
	var sig Signal
	err := s.db.QueryRow(`
		INSERT INTO signals (strategy, node, side, quantity_mw, limit_price)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id,strategy,node,side,quantity_mw,limit_price,status,reason,order_id,created_at`,
		strategy, node, side, qtyMW, limitPrice,
	).Scan(&sig.ID, &sig.Strategy, &sig.Node, &sig.Side,
		&sig.QuantityMW, &sig.LimitPrice, &sig.Status,
		&sig.Reason, &sig.OrderID, &sig.CreatedAt)
	return &sig, err
}

func (s *Store) GetByID(id int64) (*Signal, error) {
	var sig Signal
	err := s.db.QueryRow(`
		SELECT id,strategy,node,side,quantity_mw,limit_price,status,reason,order_id,created_at
		FROM signals WHERE id=$1`, id,
	).Scan(&sig.ID, &sig.Strategy, &sig.Node, &sig.Side,
		&sig.QuantityMW, &sig.LimitPrice, &sig.Status,
		&sig.Reason, &sig.OrderID, &sig.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &sig, err
}

// ClaimPending atomically transitions PENDING → status. Returns false if already claimed.
func (s *Store) ClaimPending(id int64, status Status, reason *string) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE signals SET status=$1, reason=$2 WHERE id=$3 AND status='PENDING'`,
		string(status), reason, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) SetOrderID(id int64, orderID int64) error {
	_, err := s.db.Exec(`UPDATE signals SET order_id=$1 WHERE id=$2`, orderID, id)
	return err
}

func (s *Store) LatestSubmitted(strategy, node string) (*time.Time, error) {
	var ts time.Time
	err := s.db.QueryRow(`
		SELECT created_at FROM signals
		WHERE strategy=$1 AND node=$2 AND status='SUBMITTED'
		ORDER BY created_at DESC LIMIT 1`, strategy, node,
	).Scan(&ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ts, nil
}
```

- [ ] **Step 6: Run tests**

```bash
cd strategy-engine
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./internal/signal/... -v
```

Expected: `6 tests PASS`.

- [ ] **Step 7: Commit**

```bash
git add strategy-engine/go.mod strategy-engine/go.sum strategy-engine/internal/signal/
git commit -m "feat(strategy-engine): Go module + signal model and store (TDD)"
```

---

### Task 6: riskstore + risk gate (TDD)

**Files:**
- Create: `strategy-engine/internal/riskstore/riskstore.go`
- Create: `strategy-engine/internal/gate/gate.go`
- Create: `strategy-engine/internal/gate/gate_test.go`

- [ ] **Step 1: Write riskstore.go**

Create `strategy-engine/internal/riskstore/riskstore.go`:

```go
package riskstore

import (
	"database/sql"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// LatestNetExposure returns the most recent net_exposure_mw for the node,
// or 0 if no risk snapshot exists yet.
func (s *Store) LatestNetExposure(node string) (float64, error) {
	var v float64
	err := s.db.QueryRow(`
		SELECT net_exposure_mw FROM risk_snapshots
		WHERE node=$1 ORDER BY snapshot_at DESC LIMIT 1`, node,
	).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
```

- [ ] **Step 2: Write failing gate tests**

Create `strategy-engine/internal/gate/gate_test.go`:

```go
package gate_test

import (
	"testing"
	"time"

	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/gate"
)

type mockRisk struct{ exposure float64 }

func (m *mockRisk) LatestNetExposure(_ string) (float64, error) { return m.exposure, nil }

type mockSignals struct{ latest *time.Time }

func (m *mockSignals) LatestSubmitted(_, _ string) (*time.Time, error) { return m.latest, nil }

func TestGate_AllowsWhenUnderLimit(t *testing.T) {
	g := gate.New(&mockRisk{exposure: 10.0}, &mockSignals{}, 40.0, 30)
	allowed, reason := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if !allowed {
		t.Errorf("want allowed, got skipped: %s", reason)
	}
}

func TestGate_BlocksWhenAtLimit(t *testing.T) {
	// 36 + 5 = 41 >= 40 → blocked
	g := gate.New(&mockRisk{exposure: 36.0}, &mockSignals{}, 40.0, 30)
	allowed, reason := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if allowed {
		t.Error("want blocked at limit")
	}
	if reason != "risk_limit" {
		t.Errorf("want reason=risk_limit got %s", reason)
	}
}

func TestGate_BlocksOnActiveCooldown(t *testing.T) {
	recent := time.Now().Add(-10 * time.Second) // 10s ago, cooldown=30
	g := gate.New(&mockRisk{exposure: 5.0}, &mockSignals{latest: &recent}, 40.0, 30)
	allowed, reason := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if allowed {
		t.Error("want blocked on cooldown")
	}
	if reason != "cooldown" {
		t.Errorf("want reason=cooldown got %s", reason)
	}
}

func TestGate_AllowsAfterCooldownExpires(t *testing.T) {
	old := time.Now().Add(-60 * time.Second) // 60s ago, cooldown=30 → expired
	g := gate.New(&mockRisk{exposure: 5.0}, &mockSignals{latest: &old}, 40.0, 30)
	allowed, _ := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if !allowed {
		t.Error("want allowed after cooldown expires")
	}
}

func TestGate_AllowsWithNoSubmittedHistory(t *testing.T) {
	g := gate.New(&mockRisk{exposure: 0.0}, &mockSignals{latest: nil}, 40.0, 30)
	allowed, _ := g.Check("mean_reversion", "HB_NORTH", 5.0)
	if !allowed {
		t.Error("want allowed with no prior submissions")
	}
}
```

- [ ] **Step 3: Run to verify failure**

```bash
cd strategy-engine && go test ./internal/gate/... 2>&1 | head -5
```

Expected: compile error — `gate.go` missing.

- [ ] **Step 4: Implement gate.go**

Create `strategy-engine/internal/gate/gate.go`:

```go
package gate

import "time"

type RiskQuerier interface {
	LatestNetExposure(node string) (float64, error)
}

type SignalQuerier interface {
	LatestSubmitted(strategy, node string) (*time.Time, error)
}

type Gate struct {
	risk         RiskQuerier
	signals      SignalQuerier
	posLimitMW   float64
	cooldownSecs int
}

func New(risk RiskQuerier, signals SignalQuerier, posLimitMW float64, cooldownSecs int) *Gate {
	return &Gate{risk: risk, signals: signals, posLimitMW: posLimitMW, cooldownSecs: cooldownSecs}
}

// Check returns (true, "") if the signal may be submitted, or (false, reason) if blocked.
func (g *Gate) Check(strategy, node string, quantityMW float64) (bool, string) {
	netExp, err := g.risk.LatestNetExposure(node)
	if err != nil {
		return false, "risk_query_error"
	}
	if netExp+quantityMW >= g.posLimitMW {
		return false, "risk_limit"
	}

	latest, err := g.signals.LatestSubmitted(strategy, node)
	if err != nil {
		return false, "signal_query_error"
	}
	if latest != nil {
		if time.Since(*latest).Seconds() < float64(g.cooldownSecs) {
			return false, "cooldown"
		}
	}
	return true, ""
}
```

- [ ] **Step 5: Run tests**

```bash
cd strategy-engine && go test ./internal/gate/... -v
```

Expected: `5 tests PASS`.

- [ ] **Step 6: Commit**

```bash
git add strategy-engine/internal/riskstore/ strategy-engine/internal/gate/
git commit -m "feat(strategy-engine): riskstore + risk gate with position limit and cooldown (TDD)"
```

---

### Task 7: submitter (TDD) + listener + metrics + main

**Files:**
- Create: `strategy-engine/internal/submitter/submitter.go`
- Create: `strategy-engine/internal/submitter/submitter_test.go`
- Create: `strategy-engine/internal/listener/listener.go`
- Create: `strategy-engine/metrics/metrics.go`
- Create: `strategy-engine/cmd/server/main.go`

- [ ] **Step 1: Write failing submitter tests**

Create `strategy-engine/internal/submitter/submitter_test.go`:

```go
package submitter_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/submitter"
)

func TestSubmitter_Submit_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 99})
	}))
	defer srv.Close()

	sub := submitter.New(srv.URL)
	sig := &signal.Signal{
		ID: 1, Node: "HB_NORTH", Side: "BUY",
		QuantityMW: 5.0, LimitPrice: 45.0,
	}
	orderID, err := sub.Submit(sig)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if orderID != 99 {
		t.Errorf("want orderID=99 got %d", orderID)
	}
}

func TestSubmitter_Submit_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "db error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	sub := submitter.New(srv.URL)
	sig := &signal.Signal{Node: "HB_NORTH", Side: "BUY", QuantityMW: 5.0, LimitPrice: 45.0}
	_, err := sub.Submit(sig)
	if err == nil {
		t.Error("expected error on 500 response")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd strategy-engine && go test ./internal/submitter/... 2>&1 | head -5
```

Expected: compile error.

- [ ] **Step 3: Implement submitter.go**

Create `strategy-engine/internal/submitter/submitter.go`:

```go
package submitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
)

type Submitter struct {
	orderSvcURL string
	client      *http.Client
}

func New(orderSvcURL string) *Submitter {
	return &Submitter{orderSvcURL: orderSvcURL, client: &http.Client{}}
}

type orderRequest struct {
	Node       string  `json:"node"`
	Side       string  `json:"side"`
	QuantityMW float64 `json:"quantity_mw"`
	LimitPrice float64 `json:"limit_price"`
}

type orderResponse struct {
	ID int64 `json:"id"`
}

func (s *Submitter) Submit(sig *signal.Signal) (int64, error) {
	body, _ := json.Marshal(orderRequest{
		Node:       sig.Node,
		Side:       sig.Side,
		QuantityMW: sig.QuantityMW,
		LimitPrice: sig.LimitPrice,
	})
	resp, err := s.client.Post(s.orderSvcURL+"/orders", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("order-svc returned %d", resp.StatusCode)
	}
	var out orderResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.ID, nil
}
```

- [ ] **Step 4: Run submitter tests**

```bash
cd strategy-engine && go test ./internal/submitter/... -v
```

Expected: `2 tests PASS`.

- [ ] **Step 5: Write metrics.go**

Create `strategy-engine/metrics/metrics.go`:

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SignalsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "strategy_engine_signals_received_total",
		Help: "Total signals received from strategies",
	}, []string{"strategy", "node"})

	SignalsSubmitted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "strategy_engine_signals_submitted_total",
		Help: "Total signals submitted as orders",
	}, []string{"strategy", "node"})

	SignalsSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "strategy_engine_signals_skipped_total",
		Help: "Total signals skipped by gate",
	}, []string{"strategy", "node", "reason"})
)
```

- [ ] **Step 6: Write listener.go**

Create `strategy-engine/internal/listener/listener.go`:

```go
package listener

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/gate"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/submitter"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/metrics"
)

type Listener struct {
	dbURL     string
	db        *sql.DB
	store     *signal.Store
	gate      *gate.Gate
	submitter *submitter.Submitter
}

func New(dbURL string, db *sql.DB, store *signal.Store, g *gate.Gate, sub *submitter.Submitter) *Listener {
	return &Listener{dbURL: dbURL, db: db, store: store, gate: g, submitter: sub}
}

func (l *Listener) Run() {
	onErr := func(_ pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("strategy-engine listener error: %v", err)
		}
	}
	pl := pq.NewListener(l.dbURL, 10*time.Second, time.Minute, onErr)
	if err := pl.Listen("signals"); err != nil {
		log.Fatalf("LISTEN signals: %v", err)
	}
	log.Println("strategy-engine: listening on signals")
	for n := range pl.Notify {
		if n == nil {
			continue
		}
		id, err := strconv.ParseInt(n.Extra, 10, 64)
		if err != nil {
			log.Printf("bad signal notify payload: %q", n.Extra)
			continue
		}
		l.process(id)
	}
}

func (l *Listener) process(signalID int64) {
	sig, err := l.store.GetByID(signalID)
	if err != nil || sig == nil {
		log.Printf("get signal %d: %v", signalID, err)
		return
	}

	metrics.SignalsReceived.WithLabelValues(sig.Strategy, sig.Node).Inc()

	allowed, reason := l.gate.Check(sig.Strategy, sig.Node, sig.QuantityMW)
	if !allowed {
		l.store.ClaimPending(signalID, signal.StatusSkipped, &reason)
		metrics.SignalsSkipped.WithLabelValues(sig.Strategy, sig.Node, reason).Inc()
		log.Printf("signal %d skipped: %s", signalID, reason)
		return
	}

	claimed, err := l.store.ClaimPending(signalID, signal.StatusSubmitted, nil)
	if err != nil || !claimed {
		log.Printf("signal %d already claimed or error: %v", signalID, err)
		return
	}

	orderID, err := l.submitter.Submit(sig)
	if err != nil {
		log.Printf("signal %d submit failed: %v", signalID, err)
		metrics.SignalsSkipped.WithLabelValues(sig.Strategy, sig.Node, "order_error").Inc()
		return
	}
	l.store.SetOrderID(signalID, orderID)
	metrics.SignalsSubmitted.WithLabelValues(sig.Strategy, sig.Node).Inc()
	log.Printf("signal %d submitted as order %d (%s %s %.1fMW @ %.4f)",
		signalID, orderID, sig.Strategy, sig.Side, sig.QuantityMW, sig.LimitPrice)
}
```

- [ ] **Step 7: Write main.go**

Create `strategy-engine/cmd/server/main.go`:

```go
package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/gate"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/listener"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/riskstore"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/submitter"
)

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func waitForOrderSvc(url string) {
	for {
		resp, err := http.Get(url + "/health")
		if err == nil && resp.StatusCode == 200 {
			log.Println("order-svc is healthy")
			return
		}
		log.Printf("waiting for order-svc at %s...", url)
		time.Sleep(3 * time.Second)
	}
}

func main() {
	dbURL        := getenv("DATABASE_URL",    "postgresql://postgres:postgres@localhost:5432/trade_infra")
	orderSvcURL  := getenv("ORDER_SVC_URL",   "http://localhost:18080")
	metricsAddr  := getenv("METRICS_ADDR",    ":9104")
	posLimitMW   := mustFloat(getenv("POSITION_LIMIT_MW", "40"))
	cooldownSecs := mustInt(getenv("COOLDOWN_SECS",        "30"))

	waitForOrderSvc(orderSvcURL)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	sigStore  := signal.NewStore(db)
	riskStore := riskstore.NewStore(db)
	g         := gate.New(riskStore, sigStore, posLimitMW, cooldownSecs)
	sub       := submitter.New(orderSvcURL)
	l         := listener.New(dbURL, db, sigStore, g, sub)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("ok"))
		})
		log.Printf("strategy-engine metrics on %s", metricsAddr)
		log.Fatal(http.ListenAndServe(metricsAddr, mux))
	}()

	l.Run()
}

func mustFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Fatalf("invalid float %q: %v", s, err)
	}
	return v
}

func mustInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("invalid int %q: %v", s, err)
	}
	return v
}
```

- [ ] **Step 8: Build verify**

```bash
cd strategy-engine && go build ./...
```

Expected: no errors.

- [ ] **Step 9: Run all Go tests**

```bash
cd strategy-engine
TEST_DATABASE_URL="postgresql://$(whoami)@localhost:5432/trade_infra_test?sslmode=disable" \
go test ./... -v 2>&1 | tail -20
```

Expected: signal (6), gate (5), submitter (2) = 13 tests PASS.

- [ ] **Step 10: Commit**

```bash
git add strategy-engine/
git commit -m "feat(strategy-engine): submitter, listener, metrics, main — 13 tests pass"
```

---

## Phase 4: Infrastructure

### Task 8: Python Dockerfile.strategy + strategy-engine Dockerfile + Docker Compose + Prometheus

**Files:**
- Create: `python/Dockerfile.strategy`
- Create: `strategy-engine/Dockerfile`
- Modify: `infra/docker-compose.yml`
- Modify: `infra/prometheus.yml`

- [ ] **Step 1: Write python/Dockerfile.strategy**

Create `python/Dockerfile.strategy`:

```dockerfile
FROM python:3.12-slim
RUN pip install --no-cache-dir uv
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN uv sync --no-dev
COPY strategies/ strategies/
```

- [ ] **Step 2: Write strategy-engine/Dockerfile**

Create `strategy-engine/Dockerfile`:

```dockerfile
FROM golang:1.26-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /strategy-engine ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /strategy-engine /usr/local/bin/strategy-engine
ENV ORDER_SVC_URL=http://order-svc:8080 \
    POSITION_LIMIT_MW=40 \
    COOLDOWN_SECS=30 \
    METRICS_ADDR=:9104
CMD ["strategy-engine"]
```

- [ ] **Step 3: Add services to docker-compose.yml**

Append to `infra/docker-compose.yml` (inside the `services:` block, before `prometheus:`):

```yaml
  mean-reversion:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/mean_reversion.py --node HB_NORTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  ma-crossover:
    build:
      context: ../python
      dockerfile: Dockerfile.strategy
    command: uv run python strategies/ma_crossover.py --node HB_NORTH
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  strategy-engine:
    build:
      context: ../strategy-engine
      dockerfile: Dockerfile
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra?sslmode=disable
      ORDER_SVC_URL: http://order-svc:8080
      POSITION_LIMIT_MW: "40"
      COOLDOWN_SECS: "30"
      METRICS_ADDR: ":9104"
    ports: ["9104:9104"]
    depends_on:
      postgres:
        condition: service_healthy
      order-svc:
        condition: service_started
    restart: unless-stopped
```

- [ ] **Step 4: Add strategy-engine to prometheus.yml**

Add to the `scrape_configs` list in `infra/prometheus.yml`:

```yaml
  - job_name: strategy-engine
    static_configs:
      - targets: ["strategy-engine:9104"]
```

- [ ] **Step 5: Build and verify all images build**

```bash
cd infra && docker compose build mean-reversion ma-crossover strategy-engine 2>&1 | tail -10
```

Expected: `Image infra-mean-reversion Built`, `Image infra-ma-crossover Built`, `Image infra-strategy-engine Built`.

- [ ] **Step 6: Commit**

```bash
git add python/Dockerfile.strategy strategy-engine/Dockerfile infra/docker-compose.yml infra/prometheus.yml
git commit -m "feat(infra): Docker Compose + Dockerfiles for strategy daemons and strategy-engine"
```

---

## Phase 5: Integration

### Task 9: Extend smoke test

**Files:**
- Modify: `python/smoke_test.py`

- [ ] **Step 1: Add strategy-engine health check and signal verification to smoke_test.py**

Add the following to `python/smoke_test.py` after the existing risk snapshot check:

```python
STRATEGY_ENGINE = "http://localhost:9104"

# (add to the wait_for calls at the top of main())
wait_for(f"{STRATEGY_ENGINE}/health")
print("strategy-engine healthy.")

# (add at the end of main(), after the risk snapshot check)
print("\n=== Waiting for a strategy signal ===")
deadline = time.time() + 120  # strategies need warm-up time
signal_submitted = False
while time.time() < deadline:
    cur.execute(
        "SELECT id, strategy, side, status, order_id FROM signals "
        "WHERE status='SUBMITTED' AND order_id IS NOT NULL LIMIT 1"
    )
    row = cur.fetchone()
    if row:
        sig_id, strategy, side, status, order_id = row
        print(f"Signal id={sig_id} strategy={strategy} side={side} order_id={order_id}")
        signal_submitted = True
        break
    time.sleep(3)

assert signal_submitted, "No submitted signal with order_id within 120s"
print("✓ Strategy signal submitted and order placed")
```

- [ ] **Step 2: Run full Python test suite to confirm existing tests unaffected**

```bash
cd python && uv run pytest tests/ -v 2>&1 | tail -5
```

Expected: 22 passed (unchanged).

- [ ] **Step 3: Commit**

```bash
git add python/smoke_test.py
git commit -m "test: extend smoke test to verify strategy signal submission"
```

---

## Spec Coverage Check

| Spec requirement | Covered by |
|---|---|
| signals table + strategy_configs table + seed rows | Task 1 |
| mean_reversion.py: rolling window, warm-up, BUY/SELL bands, NOTIFY | Task 3 |
| ma_crossover.py: EMA, edge-detection cross, warm-up, NOTIFY | Task 4 |
| Python base.py shared helpers (listen_ticks, emit_signal, load_config) | Task 2 |
| strategy-engine signal store: ClaimPending idempotent, SetOrderID, LatestSubmitted | Task 5 |
| Risk gate: position limit check (net_exposure + qty >= limit) | Task 6 |
| Risk gate: cooldown check (time since last submission) | Task 6 |
| riskstore: reads net_exposure_mw from risk_snapshots | Task 6 |
| Submitter: POST /orders, returns order_id | Task 7 |
| Listener: LISTEN 'signals', atomic claim, gate, submit, metrics | Task 7 |
| Metrics: received/submitted/skipped with strategy+node labels | Task 7 |
| Startup: wait for order-svc /health before starting | Task 7 |
| Docker Compose: mean-reversion, ma-crossover, strategy-engine services | Task 8 |
| Python Dockerfile.strategy + strategy-engine Dockerfile | Task 8 |
| Prometheus scrape config for strategy-engine:9104 | Task 8 |
| Integration smoke test: signal → order verified | Task 9 |
