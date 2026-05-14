# Backtesting Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a high-speed hybrid backtesting engine with a Rust core (PyO3/CMake+Corrosion), Python decorator API, FastAPI backend, and React performance analytics dashboard.

**Architecture:** Rust handles all numeric computation (indicators, fill simulation, metrics) compiled to a Python extension via PyO3. A Python `@strategy` decorator API calls the Rust engine and self-registers strategies. FastAPI serves results; Vite+React renders the equity curve and metrics dashboard.

**Tech Stack:** Rust 1.78 + PyO3 0.21, CMake + Corrosion v0.5, scikit-build-core, uv, FastAPI, yfinance, Vite + React 18 + TypeScript + Recharts, Docker (3-stage)

---

## File Map

| File | Responsibility |
|------|---------------|
| `Cargo.toml` | Rust crate manifest (`cdylib`, PyO3 dep) |
| `CMakeLists.txt` | Corrosion integration: CMake → Cargo → `.so` |
| `pyproject.toml` | scikit-build-core backend, Python deps |
| `.gitignore` | Excludes `target/`, `.so`, `node_modules/`, `.venv/` |
| `src/types.rs` | `Bar`, `Side`, `Order`, `Trade`, `EquityPoint` structs |
| `src/indicators.rs` | Pure numeric: `sma()`, `ema()`, `rsi()` on `&[f64]` |
| `src/portfolio.rs` | Cash/position state, fill recording, equity tracking |
| `src/execution.rs` | Fill price calculator (slippage, commission) |
| `src/metrics.rs` | Sharpe, Sortino, max drawdown, win rate from equity curve |
| `src/backtest.rs` | Hybrid runner: signal array → event-driven fill loop |
| `src/lib.rs` | PyO3 module: exposes `Bar`, `BacktestEngine`, `sma/ema/rsi` |
| `backtester/__init__.py` | Re-exports PyO3 types |
| `backtester/decorators.py` | `@strategy`, `@indicator` with indicator cache |
| `backtester/indicators.py` | Python-facing `sma(bars, period)` etc. wrapping Rust fns |
| `backtester/api.py` | FastAPI: `/run`, `/strategies`, `/health` + static serving |
| `strategies/__init__.py` | Empty |
| `strategies/examples.py` | SMA crossover example using `@strategy` |
| `tests/conftest.py` | `sample_bars` fixture |
| `tests/test_engine.py` | Integration tests for `BacktestEngine` via PyO3 |
| `tests/test_decorators.py` | Tests for `@strategy` and `@indicator` (mocks yfinance) |
| `tests/test_api.py` | FastAPI `TestClient` tests |
| `frontend/package.json` | Vite + React + Recharts deps |
| `frontend/vite.config.ts` | Dev proxy to FastAPI on :8000 |
| `frontend/index.html` | Entry HTML |
| `frontend/src/types.ts` | TypeScript types for API contract |
| `frontend/src/main.tsx` | React root mount |
| `frontend/src/App.tsx` | State owner, composes RunForm + MetricsCards + EquityCurve |
| `frontend/src/index.css` | Dark theme, layout |
| `frontend/src/components/RunForm.tsx` | Strategy picker + Run button |
| `frontend/src/components/MetricsCards.tsx` | Six stat tiles |
| `frontend/src/components/EquityCurve.tsx` | Recharts AreaChart (equity + drawdown) |
| `Dockerfile` | 3-stage: rust-builder, node-builder, final |
| `docker-compose.yml` | `api` service (prod) + `frontend` service (dev) |
| `README.md` | Prerequisites + dev setup + Docker instructions |

---

### Task 1: Repository Scaffold

**Files:**
- Create: `.gitignore`
- Create: `Cargo.toml`
- Create: `CMakeLists.txt`
- Create: `pyproject.toml`
- Create: `backtester/__init__.py`
- Create: `src/lib.rs` (stub only — full content in Task 6)

- [ ] **Step 1: Create `.gitignore`**

```
/target/
backtester/*.so
backtester/*.dylib
backtester/*.dll
.venv/
__pycache__/
*.egg-info/
_skbuild/
build/
dist/
node_modules/
frontend/dist/
.superpowers/
Cargo.lock
```

- [ ] **Step 2: Create `Cargo.toml`**

```toml
[package]
name = "quant_engine"
version = "0.1.0"
edition = "2021"

[lib]
name = "quant_engine"
crate-type = ["cdylib"]

[dependencies]
pyo3 = { version = "0.21", features = ["extension-module"] }
```

- [ ] **Step 3: Create `CMakeLists.txt`**

```cmake
cmake_minimum_required(VERSION 3.15)
project(quant_research_tool)

include(FetchContent)
FetchContent_Declare(
  corrosion
  GIT_REPOSITORY https://github.com/corrosion-rs/corrosion.git
  GIT_TAG v0.5
)
FetchContent_MakeAvailable(corrosion)

find_package(Python COMPONENTS Interpreter Development.Module REQUIRED)
corrosion_import_crate(MANIFEST_PATH Cargo.toml)
corrosion_set_env_vars(quant_engine
  "PYO3_PYTHON=${Python_EXECUTABLE}"
  "PYTHON_SYS_EXECUTABLE=${Python_EXECUTABLE}"
)
corrosion_install(TARGETS quant_engine DESTINATION backtester)
```

- [ ] **Step 4: Create `pyproject.toml`**

```toml
[build-system]
requires = ["scikit-build-core", "cmake"]
build-backend = "scikit_build_core.build"

[project]
name = "quant-research-tool"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = [
    "fastapi",
    "uvicorn",
    "yfinance",
    "httpx",
    "pytest",
]

[tool.scikit-build]
cmake.version = ">=3.15"
ninja.version = ">=1.10"

[tool.scikit-build.wheel]
packages = ["backtester", "strategies"]
```

- [ ] **Step 5: Create `backtester/__init__.py` (import stub — will work after build)**

```python
from . import quant_engine  # noqa: F401
from .quant_engine import Bar, BacktestEngine, BacktestResult, EquityPoint, Trade  # noqa: F401
```

- [ ] **Step 6: Create stub `src/lib.rs` (placeholder so Cargo doesn't error)**

```rust
use pyo3::prelude::*;

#[pymodule]
fn quant_engine(_py: Python<'_>, _m: &PyModule) -> PyResult<()> {
    Ok(())
}
```

- [ ] **Step 7: Commit**

```bash
git add .gitignore Cargo.toml CMakeLists.txt pyproject.toml backtester/__init__.py src/lib.rs
git commit -m "chore: repository scaffold (build system + package stub)"
```

---

### Task 2: Write Failing Engine Tests

These tests will fail with `ModuleNotFoundError` until the build runs in Task 7. Writing them first defines the contract the Rust engine must satisfy.

**Files:**
- Create: `tests/conftest.py`
- Create: `tests/test_engine.py`

- [ ] **Step 1: Create `tests/conftest.py`**

```python
import pytest

@pytest.fixture
def sample_bars():
    from backtester import quant_engine
    prices = [
        100.0, 101.0, 102.0, 103.0, 102.0, 101.0, 100.0, 101.0, 103.0, 105.0,
        107.0, 106.0, 105.0, 104.0, 106.0, 108.0, 110.0, 109.0, 108.0, 110.0,
    ]
    return [
        quant_engine.Bar(
            date=i * 86400,
            open=p,
            high=p + 1.0,
            low=p - 1.0,
            close=p,
            volume=10_000.0,
        )
        for i, p in enumerate(prices)
    ]
```

- [ ] **Step 2: Create `tests/test_engine.py`**

```python
import pytest
from backtester import quant_engine


def test_bar_fields(sample_bars):
    b = sample_bars[0]
    assert b.date == 0
    assert b.close == 100.0
    assert b.open == 100.0
    assert b.high == 101.0
    assert b.low == 99.0


def test_backtest_equity_curve_has_one_point_per_bar(sample_bars):
    engine = quant_engine.BacktestEngine(capital=10_000.0, slippage_bps=0.0, commission=0.0)
    result = engine.run(sample_bars, [0.0] * 20, "TEST")
    assert len(result.equity_curve) == 20


def test_flat_signal_no_trades(sample_bars):
    engine = quant_engine.BacktestEngine(capital=10_000.0, slippage_bps=0.0, commission=0.0)
    result = engine.run(sample_bars, [0.0] * 20, "TEST")
    assert result.metrics.num_trades == 0
    assert result.equity_curve[-1].value == pytest.approx(10_000.0, rel=1e-6)


def test_profitable_uptrend_signal(sample_bars):
    # Prices 100 → 110 over 20 bars; buy first 16, sell last 4
    engine = quant_engine.BacktestEngine(capital=10_000.0, slippage_bps=0.0, commission=0.0)
    result = engine.run(sample_bars, [1.0] * 16 + [-1.0] * 4, "TEST")
    assert result.metrics.total_return > 0


def test_mismatched_lengths_raises(sample_bars):
    engine = quant_engine.BacktestEngine(capital=10_000.0, slippage_bps=0.0, commission=0.0)
    with pytest.raises(Exception):
        engine.run(sample_bars, [1.0] * 5, "TEST")


def test_metrics_fields(sample_bars):
    engine = quant_engine.BacktestEngine(capital=10_000.0, slippage_bps=0.0, commission=0.0)
    m = engine.run(sample_bars, [1.0] * 16 + [-1.0] * 4, "TEST").metrics
    for field in ("total_return", "sharpe", "sortino", "max_drawdown", "win_rate", "num_trades"):
        assert hasattr(m, field)
    assert 0.0 <= m.max_drawdown <= 1.0
    assert 0.0 <= m.win_rate <= 1.0


def test_sma_length(sample_bars):
    closes = [b.close for b in sample_bars]
    result = quant_engine.sma(closes, 5)
    assert len(result) == len(closes)


def test_ema_length(sample_bars):
    closes = [b.close for b in sample_bars]
    result = quant_engine.ema(closes, 5)
    assert len(result) == len(closes)


def test_rsi_length(sample_bars):
    closes = [b.close for b in sample_bars]
    result = quant_engine.rsi(closes, 14)
    assert len(result) == len(closes)
```

- [ ] **Step 3: Confirm tests fail with ModuleNotFoundError (expected)**

```bash
cd /path/to/quant_research_tool
uv venv && source .venv/bin/activate
pytest tests/test_engine.py -v 2>&1 | head -20
```

Expected: `ModuleNotFoundError: No module named 'backtester.quant_engine'`

- [ ] **Step 4: Commit**

```bash
git add tests/
git commit -m "test: add failing engine integration tests (pre-build)"
```

---

### Task 3: Rust Types + Indicators

**Files:**
- Create: `src/types.rs`
- Create: `src/indicators.rs`

- [ ] **Step 1: Create `src/types.rs`**

```rust
use pyo3::prelude::*;

#[derive(Clone, Debug)]
#[pyclass]
pub struct Bar {
    #[pyo3(get, set)]
    pub date: i64,
    #[pyo3(get, set)]
    pub open: f64,
    #[pyo3(get, set)]
    pub high: f64,
    #[pyo3(get, set)]
    pub low: f64,
    #[pyo3(get, set)]
    pub close: f64,
    #[pyo3(get, set)]
    pub volume: f64,
}

#[pymethods]
impl Bar {
    #[new]
    pub fn new(date: i64, open: f64, high: f64, low: f64, close: f64, volume: f64) -> Self {
        Bar { date, open, high, low, close, volume }
    }
}

#[derive(Clone, Debug, PartialEq)]
pub enum Side { Buy, Sell }

#[derive(Clone, Debug)]
pub struct Order {
    pub date: i64,
    pub symbol: String,
    pub qty: f64,
    pub side: Side,
}

#[derive(Clone, Debug)]
#[pyclass]
pub struct Trade {
    #[pyo3(get)]
    pub entry_date: i64,
    #[pyo3(get)]
    pub exit_date: i64,
    #[pyo3(get)]
    pub symbol: String,
    #[pyo3(get)]
    pub pnl: f64,
    #[pyo3(get)]
    pub pnl_pct: f64,
}

#[derive(Clone, Debug)]
#[pyclass]
pub struct EquityPoint {
    #[pyo3(get)]
    pub date: i64,
    #[pyo3(get)]
    pub value: f64,
}
```

- [ ] **Step 2: Create `src/indicators.rs`**

```rust
pub fn sma(prices: &[f64], period: usize) -> Vec<f64> {
    let n = prices.len();
    let mut out = vec![f64::NAN; n];
    if period == 0 || period > n {
        return out;
    }
    let mut sum: f64 = prices[..period].iter().sum();
    out[period - 1] = sum / period as f64;
    for i in period..n {
        sum += prices[i] - prices[i - period];
        out[i] = sum / period as f64;
    }
    out
}

pub fn ema(prices: &[f64], period: usize) -> Vec<f64> {
    let n = prices.len();
    let mut out = vec![f64::NAN; n];
    if period == 0 || period > n {
        return out;
    }
    let k = 2.0 / (period as f64 + 1.0);
    let seed: f64 = prices[..period].iter().sum::<f64>() / period as f64;
    out[period - 1] = seed;
    for i in period..n {
        out[i] = prices[i] * k + out[i - 1] * (1.0 - k);
    }
    out
}

pub fn rsi(prices: &[f64], period: usize) -> Vec<f64> {
    let n = prices.len();
    let mut out = vec![f64::NAN; n];
    if period == 0 || n <= period {
        return out;
    }
    let changes: Vec<f64> = prices.windows(2).map(|w| w[1] - w[0]).collect();
    let (mut avg_gain, mut avg_loss) = changes[..period]
        .iter()
        .fold((0.0f64, 0.0f64), |(g, l), &c| {
            if c > 0.0 { (g + c, l) } else { (g, l + c.abs()) }
        });
    avg_gain /= period as f64;
    avg_loss /= period as f64;

    let rs = if avg_loss == 0.0 { f64::INFINITY } else { avg_gain / avg_loss };
    out[period] = 100.0 - 100.0 / (1.0 + rs);

    for i in (period + 1)..n {
        let c = changes[i - 1];
        let (gain, loss) = if c > 0.0 { (c, 0.0) } else { (0.0, c.abs()) };
        avg_gain = (avg_gain * (period as f64 - 1.0) + gain) / period as f64;
        avg_loss = (avg_loss * (period as f64 - 1.0) + loss) / period as f64;
        let rs = if avg_loss == 0.0 { f64::INFINITY } else { avg_gain / avg_loss };
        out[i] = 100.0 - 100.0 / (1.0 + rs);
    }
    out
}
```

- [ ] **Step 3: Commit**

```bash
git add src/types.rs src/indicators.rs
git commit -m "feat(rust): add Bar/Trade/EquityPoint types and SMA/EMA/RSI indicators"
```

---

### Task 4: Rust Portfolio + Execution Engine

**Files:**
- Create: `src/portfolio.rs`
- Create: `src/execution.rs`

- [ ] **Step 1: Create `src/portfolio.rs`**

```rust
use crate::types::{EquityPoint, Order, Side, Trade};

pub struct Portfolio {
    cash: f64,
    position_qty: f64,
    position_cost: f64,
    position_entry_date: i64,
    trades: Vec<Trade>,
    equity_curve: Vec<EquityPoint>,
}

impl Portfolio {
    pub fn new(capital: f64) -> Self {
        Portfolio {
            cash: capital,
            position_qty: 0.0,
            position_cost: 0.0,
            position_entry_date: 0,
            trades: Vec::new(),
            equity_curve: Vec::new(),
        }
    }

    pub fn fill(&mut self, order: &Order, fill_price: f64, commission: f64) {
        match order.side {
            Side::Buy => {
                if self.position_qty == 0.0 {
                    let cost = order.qty * fill_price + commission;
                    self.cash -= cost;
                    self.position_qty = order.qty;
                    self.position_cost = fill_price;
                    self.position_entry_date = order.date;
                }
            }
            Side::Sell => {
                if self.position_qty > 0.0 {
                    let proceeds = self.position_qty * fill_price - commission;
                    let entry_cost = self.position_qty * self.position_cost + commission;
                    let pnl = proceeds - entry_cost;
                    let pnl_pct = pnl / entry_cost;
                    self.cash += proceeds;
                    self.trades.push(Trade {
                        entry_date: self.position_entry_date,
                        exit_date: order.date,
                        symbol: order.symbol.clone(),
                        pnl,
                        pnl_pct,
                    });
                    self.position_qty = 0.0;
                    self.position_cost = 0.0;
                }
            }
        }
    }

    pub fn record_equity(&mut self, date: i64, current_price: f64) {
        let market_value = self.position_qty * current_price;
        self.equity_curve.push(EquityPoint { date, value: self.cash + market_value });
    }

    pub fn cash(&self) -> f64 { self.cash }
    pub fn trades(&self) -> &[Trade] { &self.trades }
    pub fn equity_curve(&self) -> &[EquityPoint] { &self.equity_curve }
}
```

- [ ] **Step 2: Create `src/execution.rs`**

```rust
use crate::types::Side;

pub struct ExecutionEngine {
    slippage_bps: f64,
    commission: f64,
}

impl ExecutionEngine {
    pub fn new(slippage_bps: f64, commission: f64) -> Self {
        ExecutionEngine { slippage_bps, commission }
    }

    pub fn fill_price(&self, open: f64, side: &Side) -> f64 {
        let slip = open * self.slippage_bps / 10_000.0;
        match side {
            Side::Buy => open + slip,
            Side::Sell => open - slip,
        }
    }

    pub fn commission(&self) -> f64 { self.commission }
}
```

- [ ] **Step 3: Commit**

```bash
git add src/portfolio.rs src/execution.rs
git commit -m "feat(rust): add Portfolio position tracker and ExecutionEngine fill calculator"
```

---

### Task 5: Rust Metrics + Backtest Runner

**Files:**
- Create: `src/metrics.rs`
- Create: `src/backtest.rs`

- [ ] **Step 1: Create `src/metrics.rs`**

```rust
use crate::types::{EquityPoint, Trade};

pub struct Metrics {
    pub total_return: f64,
    pub annualized_return: f64,
    pub sharpe: f64,
    pub sortino: f64,
    pub max_drawdown: f64,
    pub max_drawdown_duration: usize,
    pub win_rate: f64,
    pub avg_win: f64,
    pub avg_loss: f64,
    pub num_trades: usize,
}

pub fn compute(equity_curve: &[EquityPoint], trades: &[Trade]) -> Metrics {
    let n = equity_curve.len();
    let zero = Metrics {
        total_return: 0.0, annualized_return: 0.0, sharpe: 0.0, sortino: 0.0,
        max_drawdown: 0.0, max_drawdown_duration: 0, win_rate: 0.0,
        avg_win: 0.0, avg_loss: 0.0, num_trades: 0,
    };
    if n < 2 { return zero; }

    let initial = equity_curve[0].value;
    let final_val = equity_curve[n - 1].value;
    let total_return = (final_val - initial) / initial;

    let returns: Vec<f64> = equity_curve.windows(2)
        .map(|w| (w[1].value - w[0].value) / w[0].value)
        .collect();

    let nr = returns.len() as f64;
    let mean_r = returns.iter().sum::<f64>() / nr;
    let variance = returns.iter().map(|r| (r - mean_r).powi(2)).sum::<f64>() / nr;
    let std_dev = variance.sqrt();

    let annualized_return = (1.0 + total_return).powf(252.0 / nr) - 1.0;
    let sharpe = if std_dev == 0.0 { 0.0 } else { mean_r / std_dev * 252.0_f64.sqrt() };

    let downside_sq_sum: f64 = returns.iter()
        .filter(|&&r| r < 0.0)
        .map(|&r| r.powi(2))
        .sum();
    let downside_dev = (downside_sq_sum / nr).sqrt();
    let sortino = if downside_dev == 0.0 { 0.0 } else { mean_r / downside_dev * 252.0_f64.sqrt() };

    let mut peak = equity_curve[0].value;
    let mut max_dd = 0.0f64;
    let mut dd_start = 0usize;
    let mut cur_peak_idx = 0usize;
    let mut max_dd_dur = 0usize;

    for (i, ep) in equity_curve.iter().enumerate() {
        if ep.value > peak {
            peak = ep.value;
            cur_peak_idx = i;
        }
        let dd = (peak - ep.value) / peak;
        if dd > max_dd {
            max_dd = dd;
            dd_start = cur_peak_idx;
            max_dd_dur = i - dd_start;
        }
    }

    let num_trades = trades.len();
    let wins: Vec<f64> = trades.iter().filter(|t| t.pnl > 0.0).map(|t| t.pnl).collect();
    let losses: Vec<f64> = trades.iter().filter(|t| t.pnl <= 0.0).map(|t| t.pnl.abs()).collect();
    let win_rate = if num_trades == 0 { 0.0 } else { wins.len() as f64 / num_trades as f64 };
    let avg_win = if wins.is_empty() { 0.0 } else { wins.iter().sum::<f64>() / wins.len() as f64 };
    let avg_loss = if losses.is_empty() { 0.0 } else { losses.iter().sum::<f64>() / losses.len() as f64 };

    Metrics { total_return, annualized_return, sharpe, sortino, max_drawdown: max_dd,
               max_drawdown_duration: max_dd_dur, win_rate, avg_win, avg_loss, num_trades }
}
```

- [ ] **Step 2: Create `src/backtest.rs`**

```rust
use crate::execution::ExecutionEngine;
use crate::portfolio::Portfolio;
use crate::types::{Bar, EquityPoint, Order, Side, Trade};

pub struct BacktestConfig {
    pub capital: f64,
    pub slippage_bps: f64,
    pub commission: f64,
    pub symbol: String,
}

pub struct BacktestResult {
    pub equity_curve: Vec<EquityPoint>,
    pub trades: Vec<Trade>,
}

pub fn run(bars: &[Bar], signals: &[f64], config: &BacktestConfig) -> BacktestResult {
    assert_eq!(bars.len(), signals.len(), "bars and signals must have equal length");

    let engine = ExecutionEngine::new(config.slippage_bps, config.commission);
    let mut portfolio = Portfolio::new(config.capital);
    let mut pending_order: Option<Order> = None;
    let mut prev_signal = 0.0f64;

    for (i, bar) in bars.iter().enumerate() {
        // Fill any order queued from the previous bar (fill at this bar's open)
        if let Some(ref order) = pending_order {
            let fill_price = engine.fill_price(bar.open, &order.side);
            portfolio.fill(order, fill_price, engine.commission());
        }
        pending_order = None;

        // Record equity using today's close
        portfolio.record_equity(bar.date, bar.close);

        // Detect signal transitions, queue order for next bar's open
        let sig = signals[i];
        if sig > 0.0 && prev_signal <= 0.0 && portfolio.cash() > 0.0 {
            let qty = (portfolio.cash() * 0.99) / bar.close;
            if qty > 0.0 {
                pending_order = Some(Order {
                    date: bar.date,
                    symbol: config.symbol.clone(),
                    qty,
                    side: Side::Buy,
                });
            }
        } else if sig <= 0.0 && prev_signal > 0.0 {
            pending_order = Some(Order {
                date: bar.date,
                symbol: config.symbol.clone(),
                qty: 0.0, // portfolio.fill sells entire position when Side::Sell
                side: Side::Sell,
            });
        }
        prev_signal = sig;
    }

    BacktestResult {
        equity_curve: portfolio.equity_curve().to_vec(),
        trades: portfolio.trades().to_vec(),
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add src/metrics.rs src/backtest.rs
git commit -m "feat(rust): add metrics computation and hybrid backtest runner"
```

---

### Task 6: PyO3 Bindings

**Files:**
- Modify: `src/lib.rs` (replace stub with full implementation)

- [ ] **Step 1: Replace `src/lib.rs` with full PyO3 module**

```rust
mod backtest;
mod execution;
mod indicators;
mod metrics;
mod portfolio;
pub mod types;

use pyo3::prelude::*;
use types::{Bar, EquityPoint, Trade};

#[pyclass]
#[derive(Clone)]
pub struct MetricsPy {
    #[pyo3(get)] pub total_return: f64,
    #[pyo3(get)] pub annualized_return: f64,
    #[pyo3(get)] pub sharpe: f64,
    #[pyo3(get)] pub sortino: f64,
    #[pyo3(get)] pub max_drawdown: f64,
    #[pyo3(get)] pub max_drawdown_duration: usize,
    #[pyo3(get)] pub win_rate: f64,
    #[pyo3(get)] pub avg_win: f64,
    #[pyo3(get)] pub avg_loss: f64,
    #[pyo3(get)] pub num_trades: usize,
}

#[pyclass]
pub struct BacktestResult {
    #[pyo3(get)] pub equity_curve: Vec<EquityPoint>,
    #[pyo3(get)] pub trades: Vec<Trade>,
    #[pyo3(get)] pub metrics: MetricsPy,
}

#[pyclass]
pub struct BacktestEngine {
    capital: f64,
    slippage_bps: f64,
    commission: f64,
}

#[pymethods]
impl BacktestEngine {
    #[new]
    fn new(capital: f64, slippage_bps: f64, commission: f64) -> Self {
        BacktestEngine { capital, slippage_bps, commission }
    }

    fn run(&self, bars: Vec<Bar>, signals: Vec<f64>, symbol: &str) -> PyResult<BacktestResult> {
        if bars.len() != signals.len() {
            return Err(pyo3::exceptions::PyValueError::new_err(
                format!("bars ({}) and signals ({}) must have equal length", bars.len(), signals.len()),
            ));
        }
        let config = backtest::BacktestConfig {
            capital: self.capital,
            slippage_bps: self.slippage_bps,
            commission: self.commission,
            symbol: symbol.to_string(),
        };
        let result = backtest::run(&bars, &signals, &config);
        let m = metrics::compute(&result.equity_curve, &result.trades);
        Ok(BacktestResult {
            equity_curve: result.equity_curve,
            trades: result.trades,
            metrics: MetricsPy {
                total_return: m.total_return,
                annualized_return: m.annualized_return,
                sharpe: m.sharpe,
                sortino: m.sortino,
                max_drawdown: m.max_drawdown,
                max_drawdown_duration: m.max_drawdown_duration,
                win_rate: m.win_rate,
                avg_win: m.avg_win,
                avg_loss: m.avg_loss,
                num_trades: m.num_trades,
            },
        })
    }
}

#[pyfunction]
fn sma(prices: Vec<f64>, period: usize) -> Vec<f64> {
    indicators::sma(&prices, period)
}

#[pyfunction]
fn ema(prices: Vec<f64>, period: usize) -> Vec<f64> {
    indicators::ema(&prices, period)
}

#[pyfunction]
fn rsi(prices: Vec<f64>, period: usize) -> Vec<f64> {
    indicators::rsi(&prices, period)
}

#[pymodule]
fn quant_engine(_py: Python<'_>, m: &PyModule) -> PyResult<()> {
    m.add_class::<Bar>()?;
    m.add_class::<EquityPoint>()?;
    m.add_class::<Trade>()?;
    m.add_class::<MetricsPy>()?;
    m.add_class::<BacktestEngine>()?;
    m.add_class::<BacktestResult>()?;
    m.add_function(wrap_pyfunction!(sma, m)?)?;
    m.add_function(wrap_pyfunction!(ema, m)?)?;
    m.add_function(wrap_pyfunction!(rsi, m)?)?;
    Ok(())
}
```

- [ ] **Step 2: Commit**

```bash
git add src/lib.rs
git commit -m "feat(rust): complete PyO3 bindings — BacktestEngine, Bar, MetricsPy, sma/ema/rsi"
```

---

### Task 7: Build + Verify Engine Tests Pass

**Files:** No new files — this is a build + test step.

- [ ] **Step 1: Create venv and install build deps**

```bash
uv venv
source .venv/bin/activate   # Windows: .venv\Scripts\activate
uv pip install scikit-build-core cmake ninja
```

- [ ] **Step 2: Build the Rust extension**

This triggers: scikit-build-core → CMake → FetchContent (downloads Corrosion on first run) → `cargo build --release` → installs `.so` into `backtester/`

```bash
uv pip install -e . --no-build-isolation
```

Expected: after ~2-3 minutes, you see a `quant_engine.cpython-3XX-...so` file in `backtester/`.

```bash
ls backtester/*.so
```

Expected: `backtester/quant_engine.cpython-312-darwin.so` (or similar for your platform)

- [ ] **Step 3: Install remaining runtime deps**

```bash
uv pip install fastapi uvicorn yfinance httpx pytest
```

- [ ] **Step 4: Run engine tests**

```bash
pytest tests/test_engine.py -v
```

Expected output:
```
tests/test_engine.py::test_bar_fields PASSED
tests/test_engine.py::test_backtest_equity_curve_has_one_point_per_bar PASSED
tests/test_engine.py::test_flat_signal_no_trades PASSED
tests/test_engine.py::test_profitable_uptrend_signal PASSED
tests/test_engine.py::test_mismatched_lengths_raises PASSED
tests/test_engine.py::test_metrics_fields PASSED
tests/test_engine.py::test_sma_length PASSED
tests/test_engine.py::test_ema_length PASSED
tests/test_engine.py::test_rsi_length PASSED
9 passed
```

If any test fails, diagnose with `pytest tests/test_engine.py -v --tb=short` before proceeding.

- [ ] **Step 5: Commit**

```bash
git add backtester/*.so   # .so is gitignored so skip this; no files to add
git commit --allow-empty -m "chore: Rust engine build verified — all engine tests pass"
```

---

### Task 8: Python Indicator Helpers

**Files:**
- Create: `backtester/indicators.py`

- [ ] **Step 1: Create `backtester/indicators.py`**

```python
from backtester import quant_engine as _qe
from backtester.decorators import indicator


@indicator
def sma(bars, period: int) -> list[float]:
    return _qe.sma([b.close for b in bars], period)


@indicator
def ema(bars, period: int) -> list[float]:
    return _qe.ema([b.close for b in bars], period)


@indicator
def rsi(bars, period: int) -> list[float]:
    return _qe.rsi([b.close for b in bars], period)
```

Note: `backtester/decorators.py` must exist before this file is importable (created in Task 9). The `@indicator` decorator is defined there.

- [ ] **Step 2: Commit**

```bash
git add backtester/indicators.py
git commit -m "feat(python): add sma/ema/rsi indicator helpers wrapping Rust fns"
```

---

### Task 9: Python Decorators + Tests

**Files:**
- Create: `backtester/decorators.py`
- Create: `tests/test_decorators.py`

- [ ] **Step 1: Write failing tests first — `tests/test_decorators.py`**

```python
import pytest
import pandas as pd
from unittest.mock import patch


def _mock_df():
    dates = pd.date_range("2020-01-02", periods=10, freq="B")
    prices = [100.0, 101.0, 102.0, 103.0, 102.0, 101.0, 103.0, 105.0, 104.0, 106.0]
    return pd.DataFrame(
        {"Open": prices, "High": [p + 1 for p in prices],
         "Low": [p - 1 for p in prices], "Close": prices, "Volume": [1000.0] * 10},
        index=dates,
    )


def test_strategy_wraps_and_returns_result():
    with patch("yfinance.download", return_value=_mock_df()):
        from backtester.decorators import strategy

        @strategy(symbol="AAPL", start="2020-01-01", end="2020-01-15", capital=10_000)
        def _test_strat(bars):
            return [1.0] * len(bars)

        result = _test_strat()
        assert hasattr(result, "equity_curve")
        assert hasattr(result, "metrics")
        assert hasattr(result, "trades")
        assert len(result.equity_curve) == 10


def test_strategy_self_registers():
    with patch("yfinance.download", return_value=_mock_df()):
        from backtester.decorators import strategy, _REGISTRY

        @strategy(symbol="AAPL", start="2020-01-01", end="2020-01-15")
        def _reg_strat(bars):
            return [0.0] * len(bars)

        assert "_reg_strat" in _REGISTRY


def test_indicator_caches():
    from backtester.decorators import indicator, _clear_indicator_cache
    from backtester import quant_engine

    _clear_indicator_cache()
    call_count = 0

    @indicator
    def _cached_fn(bars, period):
        nonlocal call_count
        call_count += 1
        return [b.close for b in bars[:period]]

    bars = [quant_engine.Bar(i * 86400, 100.0, 101.0, 99.0, 100.0, 1000.0) for i in range(10)]
    _cached_fn(bars, 5)
    _cached_fn(bars, 5)
    assert call_count == 1  # second call should hit cache


def test_indicator_cache_cleared_between_runs():
    with patch("yfinance.download", return_value=_mock_df()):
        from backtester.decorators import strategy, indicator, _REGISTRY
        from backtester import quant_engine

        call_count = 0

        @indicator
        def _counting_indicator(bars, period):
            nonlocal call_count
            call_count += 1
            return [b.close for b in bars][:period]

        @strategy(symbol="AAPL", start="2020-01-01", end="2020-01-15", capital=10_000)
        def _double_run_strat(bars):
            _counting_indicator(bars, 3)
            return [0.0] * len(bars)

        _double_run_strat()
        first_count = call_count
        _double_run_strat()
        # Cache should be cleared between runs; indicator called again in second run
        assert call_count == first_count + 1
```

- [ ] **Step 2: Run to confirm failure**

```bash
pytest tests/test_decorators.py -v 2>&1 | head -20
```

Expected: `ImportError: cannot import name 'strategy' from 'backtester.decorators'` (module doesn't exist yet)

- [ ] **Step 3: Create `backtester/decorators.py`**

```python
import functools
from typing import Callable

import yfinance as yf

from backtester import quant_engine as _qe

_REGISTRY: dict[str, Callable] = {}
_INDICATOR_CACHE: dict = {}


def _clear_indicator_cache() -> None:
    _INDICATOR_CACHE.clear()


def indicator(fn: Callable) -> Callable:
    @functools.wraps(fn)
    def wrapper(bars, *args):
        key = (id(bars), fn.__name__, args)
        if key not in _INDICATOR_CACHE:
            _INDICATOR_CACHE[key] = fn(bars, *args)
        return _INDICATOR_CACHE[key]
    return wrapper


def strategy(
    symbol: str,
    start: str,
    end: str,
    capital: float = 100_000.0,
    slippage_bps: float = 5.0,
    commission: float = 1.0,
) -> Callable:
    def decorator(fn: Callable) -> Callable:
        @functools.wraps(fn)
        def wrapper():
            _clear_indicator_cache()
            df = yf.download(symbol, start=start, end=end, auto_adjust=True, progress=False)
            if df.empty:
                raise ValueError(f"No data returned for {symbol} {start}–{end}")
            bars = [
                _qe.Bar(
                    date=int(ts.timestamp()),
                    open=float(row["Open"]),
                    high=float(row["High"]),
                    low=float(row["Low"]),
                    close=float(row["Close"]),
                    volume=float(row["Volume"]),
                )
                for ts, row in df.iterrows()
            ]
            signals = fn(bars)
            engine = _qe.BacktestEngine(capital, slippage_bps, commission)
            return engine.run(bars, signals, symbol)

        wrapper._strategy_meta = {
            "symbol": symbol, "start": start, "end": end, "capital": capital,
        }
        _REGISTRY[fn.__name__] = wrapper
        return wrapper

    return decorator
```

- [ ] **Step 4: Run tests**

```bash
pytest tests/test_decorators.py -v
```

Expected: `4 passed`

- [ ] **Step 5: Commit**

```bash
git add backtester/decorators.py tests/test_decorators.py
git commit -m "feat(python): @strategy and @indicator decorators with indicator cache"
```

---

### Task 10: FastAPI App + Example Strategy + Tests

**Files:**
- Create: `strategies/__init__.py`
- Create: `strategies/examples.py`
- Create: `backtester/api.py`
- Create: `tests/test_api.py`

- [ ] **Step 1: Write failing API tests — `tests/test_api.py`**

```python
import pandas as pd
from unittest.mock import patch
from fastapi.testclient import TestClient


def _mock_df():
    dates = pd.date_range("2020-01-02", periods=10, freq="B")
    prices = [100.0, 101.0, 102.0, 103.0, 102.0, 101.0, 103.0, 105.0, 104.0, 106.0]
    return pd.DataFrame(
        {"Open": prices, "High": [p + 1 for p in prices],
         "Low": [p - 1 for p in prices], "Close": prices, "Volume": [1000.0] * 10},
        index=dates,
    )


def test_health():
    from backtester.api import app
    client = TestClient(app)
    assert client.get("/health").json() == {"status": "ok"}


def test_strategies_returns_list():
    from backtester.api import app
    client = TestClient(app)
    resp = client.get("/strategies")
    assert resp.status_code == 200
    assert isinstance(resp.json(), list)


def test_run_unknown_returns_404():
    from backtester.api import app
    client = TestClient(app)
    resp = client.post("/run", json={"strategy_name": "does_not_exist"})
    assert resp.status_code == 404


def test_run_registered_strategy():
    with patch("yfinance.download", return_value=_mock_df()):
        from backtester.decorators import strategy, _REGISTRY
        from backtester.api import app

        @strategy(symbol="AAPL", start="2020-01-01", end="2020-01-15", capital=10_000)
        def _api_test_strat(bars):
            return [1.0] * len(bars)

        client = TestClient(app)
        with patch("yfinance.download", return_value=_mock_df()):
            resp = client.post("/run", json={"strategy_name": "_api_test_strat"})

        assert resp.status_code == 200
        body = resp.json()
        assert "equity_curve" in body
        assert "metrics" in body
        assert "trades" in body
        assert isinstance(body["equity_curve"], list)
        assert "total_return" in body["metrics"]
```

- [ ] **Step 2: Run to confirm failure**

```bash
pytest tests/test_api.py -v 2>&1 | head -10
```

Expected: `ImportError: cannot import name 'app' from 'backtester.api'`

- [ ] **Step 3: Create `strategies/__init__.py`**

Empty file:
```python
```

- [ ] **Step 4: Create `strategies/examples.py`**

```python
from backtester.decorators import strategy
from backtester.indicators import ema, sma


@strategy(symbol="AAPL", start="2020-01-01", end="2023-12-31", capital=100_000)
def sma_crossover(bars):
    """Buy when 10-day SMA crosses above 50-day SMA; sell when it crosses below."""
    fast = sma(bars, 10)
    slow = sma(bars, 50)
    return [
        1.0 if (f == f and s == s and f > s) else  # NaN-safe: f==f is False for NaN
        -1.0 if (f == f and s == s and f < s) else
        0.0
        for f, s in zip(fast, slow)
    ]


@strategy(symbol="SPY", start="2018-01-01", end="2023-12-31", capital=50_000)
def ema_momentum(bars):
    """Buy when price is above 20-day EMA; sell when below."""
    trend = ema(bars, 20)
    return [
        1.0 if (t == t and b.close > t) else -1.0
        for b, t in zip(bars, trend)
    ]
```

- [ ] **Step 5: Create `backtester/api.py`**

```python
import importlib
import os
from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from backtester import decorators

app = FastAPI(title="Backtesting Engine")

# Auto-import strategy module so strategies self-register
_strategy_module = os.getenv("STRATEGY_MODULE", "strategies.examples")
try:
    importlib.import_module(_strategy_module)
except ImportError as exc:
    import warnings
    warnings.warn(f"Strategy module '{_strategy_module}' not found: {exc}")


class RunRequest(BaseModel):
    strategy_name: str


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/strategies")
def list_strategies():
    return list(decorators._REGISTRY.keys())


@app.post("/run")
def run_strategy(req: RunRequest):
    fn = decorators._REGISTRY.get(req.strategy_name)
    if fn is None:
        raise HTTPException(status_code=404, detail=f"Strategy '{req.strategy_name}' not found")
    result = fn()
    return {
        "equity_curve": [{"date": ep.date, "value": ep.value} for ep in result.equity_curve],
        "metrics": {
            "total_return": result.metrics.total_return,
            "annualized_return": result.metrics.annualized_return,
            "sharpe": result.metrics.sharpe,
            "sortino": result.metrics.sortino,
            "max_drawdown": result.metrics.max_drawdown,
            "win_rate": result.metrics.win_rate,
            "num_trades": result.metrics.num_trades,
        },
        "trades": [
            {
                "entry_date": t.entry_date,
                "exit_date": t.exit_date,
                "symbol": t.symbol,
                "pnl": t.pnl,
                "pnl_pct": t.pnl_pct,
            }
            for t in result.trades
        ],
    }


# Serve compiled React app in production
_dist = Path(__file__).parent / "static"
if _dist.exists():
    app.mount("/assets", StaticFiles(directory=_dist / "assets"), name="assets")

    @app.get("/{full_path:path}")
    def serve_frontend(full_path: str):
        return FileResponse(_dist / "index.html")
```

- [ ] **Step 6: Run API tests**

```bash
pytest tests/test_api.py -v
```

Expected: `4 passed`

- [ ] **Step 7: Run full test suite**

```bash
pytest tests/ -v
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add backtester/api.py strategies/ tests/test_api.py
git commit -m "feat(python): FastAPI app with /run /strategies /health + example strategies"
```

---

### Task 11: React Scaffold

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/tsconfig.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/index.html`
- Create: `frontend/src/main.tsx`

- [ ] **Step 1: Create `frontend/package.json`**

```json
{
  "name": "quant-frontend",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "recharts": "^2.10.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",
    "@vitejs/plugin-react": "^4.2.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0"
  }
}
```

- [ ] **Step 2: Create `frontend/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "jsx": "react-jsx",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noEmit": true,
    "isolatedModules": true
  },
  "include": ["src"]
}
```

- [ ] **Step 3: Create `frontend/vite.config.ts`**

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/run': 'http://localhost:8000',
      '/strategies': 'http://localhost:8000',
      '/health': 'http://localhost:8000',
    },
  },
})
```

- [ ] **Step 4: Create `frontend/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Backtesting Engine</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 5: Create `frontend/src/main.tsx`**

```typescript
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

- [ ] **Step 6: Install deps and verify scaffold compiles**

```bash
cd frontend
npm install
npm run build 2>&1 | tail -5
```

Expected: build fails with `Cannot find module './App'` — that's fine, it means the scaffold is wired correctly.

- [ ] **Step 7: Commit**

```bash
cd ..
git add frontend/
git commit -m "feat(frontend): Vite + React + TypeScript scaffold with dev proxy"
```

---

### Task 12: TypeScript Types + RunForm

**Files:**
- Create: `frontend/src/types.ts`
- Create: `frontend/src/components/RunForm.tsx`

- [ ] **Step 1: Create `frontend/src/types.ts`**

```typescript
export type EquityPoint = {
  date: number;   // Unix timestamp (seconds)
  value: number;
};

export type Metrics = {
  total_return: number;
  annualized_return: number;
  sharpe: number;
  sortino: number;
  max_drawdown: number;
  win_rate: number;
  num_trades: number;
};

export type Trade = {
  entry_date: number;
  exit_date: number;
  symbol: string;
  pnl: number;
  pnl_pct: number;
};

export type BacktestResult = {
  equity_curve: EquityPoint[];
  metrics: Metrics;
  trades: Trade[];
};
```

- [ ] **Step 2: Create `frontend/src/components/RunForm.tsx`**

```typescript
import { useEffect, useState } from 'react'

interface Props {
  onRun: (strategyName: string) => void
  loading: boolean
}

export default function RunForm({ onRun, loading }: Props) {
  const [strategies, setStrategies] = useState<string[]>([])
  const [selected, setSelected] = useState('')

  useEffect(() => {
    fetch('/strategies')
      .then(r => r.json())
      .then((names: string[]) => {
        setStrategies(names)
        if (names.length > 0) setSelected(names[0])
      })
      .catch(console.error)
  }, [])

  return (
    <div className="run-form">
      <label htmlFor="strategy-select">Strategy</label>
      <select
        id="strategy-select"
        value={selected}
        onChange={e => setSelected(e.target.value)}
        disabled={loading}
      >
        {strategies.length === 0 && <option value="">No strategies loaded</option>}
        {strategies.map(s => (
          <option key={s} value={s}>{s}</option>
        ))}
      </select>
      <button
        onClick={() => selected && onRun(selected)}
        disabled={loading || !selected}
      >
        {loading ? 'Running…' : 'Run ▶'}
      </button>
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/RunForm.tsx
git commit -m "feat(frontend): TypeScript API types and RunForm component"
```

---

### Task 13: MetricsCards Component

**Files:**
- Create: `frontend/src/components/MetricsCards.tsx`

- [ ] **Step 1: Create `frontend/src/components/MetricsCards.tsx`**

```typescript
import type { Metrics } from '../types'

interface Props { metrics: Metrics }

const pct = (n: number): string => `${(n * 100).toFixed(2)}%`
const dec = (n: number): string => n.toFixed(2)

export default function MetricsCards({ metrics }: Props) {
  const cards = [
    {
      label: 'Total Return',
      value: pct(metrics.total_return),
      positive: metrics.total_return >= 0,
    },
    {
      label: 'Sharpe Ratio',
      value: dec(metrics.sharpe),
      positive: metrics.sharpe >= 1,
    },
    {
      label: 'Sortino Ratio',
      value: dec(metrics.sortino),
      positive: metrics.sortino >= 1,
    },
    {
      label: 'Max Drawdown',
      value: pct(metrics.max_drawdown),
      positive: metrics.max_drawdown < 0.1,
    },
    {
      label: 'Win Rate',
      value: pct(metrics.win_rate),
      positive: metrics.win_rate >= 0.5,
    },
    {
      label: '# Trades',
      value: String(metrics.num_trades),
      positive: true,
    },
  ]

  return (
    <div className="metrics-cards">
      {cards.map(c => (
        <div key={c.label} className={`metric-card ${c.positive ? 'positive' : 'negative'}`}>
          <span className="metric-label">{c.label}</span>
          <span className="metric-value">{c.value}</span>
        </div>
      ))}
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/MetricsCards.tsx
git commit -m "feat(frontend): MetricsCards component — six stat tiles"
```

---

### Task 14: EquityCurve + App.tsx + CSS

**Files:**
- Create: `frontend/src/components/EquityCurve.tsx`
- Create: `frontend/src/App.tsx`
- Create: `frontend/src/index.css`

- [ ] **Step 1: Create `frontend/src/components/EquityCurve.tsx`**

```typescript
import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { EquityPoint } from '../types'

interface Props { data: EquityPoint[] }

function computeDrawdownPct(data: EquityPoint[]): number[] {
  let peak = data[0]?.value ?? 0
  return data.map(ep => {
    if (ep.value > peak) peak = ep.value
    return peak > 0 ? +((( ep.value - peak) / peak) * 100).toFixed(2) : 0
  })
}

const fmtDate = (ts: number): string =>
  new Date(ts * 1000).toLocaleDateString('en-US', { month: 'short', year: '2-digit' })

export default function EquityCurve({ data }: Props) {
  const drawdowns = computeDrawdownPct(data)
  const chartData = data.map((ep, i) => ({
    date: fmtDate(ep.date),
    equity: +ep.value.toFixed(2),
    drawdown: drawdowns[i],
  }))

  return (
    <div className="chart-container">
      <h3>Equity Curve &amp; Drawdown</h3>
      <ResponsiveContainer width="100%" height={340}>
        <AreaChart data={chartData} margin={{ top: 10, right: 40, left: 10, bottom: 0 }}>
          <defs>
            <linearGradient id="colorEquity" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#22c55e" stopOpacity={0.25} />
              <stop offset="95%" stopColor="#22c55e" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="colorDD" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#f87171" stopOpacity={0.25} />
              <stop offset="95%" stopColor="#f87171" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="#1e2a3a" />
          <XAxis dataKey="date" tick={{ fontSize: 11, fill: '#64748b' }} tickLine={false} />
          <YAxis
            yAxisId="equity"
            orientation="left"
            tick={{ fontSize: 11, fill: '#64748b' }}
            tickFormatter={v => `$${(v / 1000).toFixed(0)}k`}
            tickLine={false}
          />
          <YAxis
            yAxisId="drawdown"
            orientation="right"
            tick={{ fontSize: 11, fill: '#64748b' }}
            tickFormatter={v => `${v}%`}
            tickLine={false}
          />
          <Tooltip
            contentStyle={{ background: '#1e2336', border: '1px solid #2a3050', borderRadius: 6 }}
            labelStyle={{ color: '#94a3b8', fontSize: 11 }}
          />
          <Legend wrapperStyle={{ fontSize: 12, paddingTop: 8 }} />
          <Area
            yAxisId="equity"
            type="monotone"
            dataKey="equity"
            name="Portfolio Value ($)"
            stroke="#22c55e"
            strokeWidth={2}
            fill="url(#colorEquity)"
          />
          <Area
            yAxisId="drawdown"
            type="monotone"
            dataKey="drawdown"
            name="Drawdown (%)"
            stroke="#f87171"
            strokeWidth={1.5}
            fill="url(#colorDD)"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
```

- [ ] **Step 2: Create `frontend/src/App.tsx`**

```typescript
import { useState } from 'react'
import EquityCurve from './components/EquityCurve'
import MetricsCards from './components/MetricsCards'
import RunForm from './components/RunForm'
import type { BacktestResult } from './types'
import './index.css'

export default function App() {
  const [result, setResult] = useState<BacktestResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleRun = async (strategyName: string) => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ strategy_name: strategyName }),
      })
      if (!res.ok) {
        const msg = await res.text()
        throw new Error(`${res.status}: ${msg}`)
      }
      setResult(await res.json())
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="app">
      <header>
        <h1>Backtesting Engine</h1>
        <p className="subtitle">Rust-powered · PyO3 bindings · Hybrid execution</p>
      </header>
      <RunForm onRun={handleRun} loading={loading} />
      {error && <p className="error">{error}</p>}
      {result && (
        <>
          <MetricsCards metrics={result.metrics} />
          <EquityCurve data={result.equity_curve} />
        </>
      )}
    </div>
  )
}
```

- [ ] **Step 3: Create `frontend/src/index.css`**

```css
*, *::before, *::after { box-sizing: border-box; }

body {
  margin: 0;
  background: #0a0e1a;
  color: #e2e8f0;
  font-family: 'Inter', system-ui, -apple-system, sans-serif;
  -webkit-font-smoothing: antialiased;
}

.app {
  max-width: 1100px;
  margin: 0 auto;
  padding: 2rem 1.5rem;
}

header { margin-bottom: 2rem; }
header h1 { margin: 0 0 0.25rem; font-size: 1.5rem; font-weight: 700; color: #f1f5f9; }
header .subtitle { margin: 0; font-size: 0.8rem; color: #475569; letter-spacing: 0.04em; }

.run-form {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 2rem;
  flex-wrap: wrap;
}

.run-form label { font-size: 0.8rem; color: #64748b; }

.run-form select {
  background: #131929;
  border: 1px solid #1e2a3a;
  color: #e2e8f0;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  font-size: 0.875rem;
  min-width: 180px;
  cursor: pointer;
}

.run-form button {
  background: #2563eb;
  color: #fff;
  border: none;
  padding: 0.5rem 1.5rem;
  border-radius: 6px;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.875rem;
  transition: background 0.15s;
}

.run-form button:disabled { opacity: 0.45; cursor: not-allowed; }
.run-form button:hover:not(:disabled) { background: #1d4ed8; }

.metrics-cards {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 0.875rem;
  margin-bottom: 1.5rem;
}

@media (max-width: 900px) { .metrics-cards { grid-template-columns: repeat(3, 1fr); } }
@media (max-width: 500px) { .metrics-cards { grid-template-columns: repeat(2, 1fr); } }

.metric-card {
  background: #131929;
  border: 1px solid #1e2a3a;
  border-radius: 8px;
  padding: 1rem 0.875rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.metric-label {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #475569;
}

.metric-value { font-size: 1.25rem; font-weight: 700; }
.metric-card.positive .metric-value { color: #22c55e; }
.metric-card.negative .metric-value { color: #f87171; }

.chart-container {
  background: #131929;
  border: 1px solid #1e2a3a;
  border-radius: 10px;
  padding: 1.5rem;
}

.chart-container h3 {
  margin: 0 0 1.25rem;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #475569;
}

.error {
  color: #f87171;
  background: #1f0d0d;
  border: 1px solid #450a0a;
  border-radius: 6px;
  padding: 0.75rem 1rem;
  font-size: 0.875rem;
  margin-bottom: 1rem;
}
```

- [ ] **Step 4: Build frontend to verify no TypeScript errors**

```bash
cd frontend
npm run build
```

Expected: `dist/` directory created, no TypeScript errors.

- [ ] **Step 5: Smoke test end-to-end (requires FastAPI running)**

Start backend in one terminal:
```bash
uvicorn backtester.api:app --reload --port 8000
```

Start frontend in another:
```bash
cd frontend && npm run dev
```

Open `http://localhost:5173`, select a strategy, click **Run ▶**. Confirm equity curve and metrics appear. Check browser console for errors.

- [ ] **Step 6: Commit**

```bash
cd ..
git add frontend/src/
git commit -m "feat(frontend): EquityCurve, App, dark theme CSS — dashboard complete"
```

---

### Task 15: Dockerfile + docker-compose + README

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `README.md`

- [ ] **Step 1: Create `Dockerfile`**

```dockerfile
# ── Stage 1: Build Rust extension ────────────────────────────────────────────
FROM rust:1.78-slim AS rust-builder

RUN apt-get update && apt-get install -y \
    cmake ninja-build python3 python3-dev python3-pip curl \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir uv

WORKDIR /app
COPY Cargo.toml pyproject.toml CMakeLists.txt ./
COPY src/ ./src/
COPY backtester/ ./backtester/
COPY strategies/ ./strategies/

RUN uv pip install --system scikit-build-core cmake
RUN uv pip install --system -e . --no-build-isolation

# ── Stage 2: Build React frontend ────────────────────────────────────────────
FROM node:20-slim AS node-builder

WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 3: Production image ─────────────────────────────────────────────────
FROM python:3.12-slim AS final

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir uv

WORKDIR /app

# Copy built .so and Python source
COPY --from=rust-builder /app/backtester/ ./backtester/
COPY --from=rust-builder /app/strategies/ ./strategies/

# Copy compiled React app into FastAPI's static dir
COPY --from=node-builder /frontend/dist/ ./backtester/static/

# Install only runtime Python deps (no Rust build needed)
RUN uv pip install --system fastapi uvicorn yfinance

EXPOSE 8000
CMD ["uvicorn", "backtester.api:app", "--host", "0.0.0.0", "--port", "8000"]
```

- [ ] **Step 2: Create `docker-compose.yml`**

```yaml
services:
  api:
    build:
      context: .
      target: final
    ports:
      - "8000:8000"
    environment:
      - STRATEGY_MODULE=strategies.examples

  # Development-only: hot-reload FastAPI + Vite dev server
  api-dev:
    build:
      context: .
      target: rust-builder
    ports:
      - "8000:8000"
    volumes:
      - ./backtester:/app/backtester
      - ./strategies:/app/strategies
    command: uvicorn backtester.api:app --host 0.0.0.0 --port 8000 --reload
    environment:
      - STRATEGY_MODULE=strategies.examples
    profiles:
      - dev
```

- [ ] **Step 3: Create `README.md`**

```markdown
# quant_research_tool

High-speed backtesting engine: Rust core + PyO3 Python bindings + React dashboard.

## Architecture

- **Rust core** (`src/`): SMA/EMA/RSI indicators, hybrid backtest runner (vectorized signals → event-driven fills), Sharpe/Sortino/drawdown metrics
- **PyO3 bindings** (`src/lib.rs`): Exposes `BacktestEngine`, `Bar`, `sma/ema/rsi` to Python
- **Build system**: CMake + Corrosion (integrates Cargo into CMake), scikit-build-core Python backend
- **Python API** (`backtester/`): `@strategy` / `@indicator` decorators, FastAPI backend
- **Frontend** (`frontend/`): Vite + React + Recharts — equity curve and metrics dashboard

## Prerequisites

- [Rust](https://rustup.rs/) 1.78+
- CMake 3.15+
- [uv](https://github.com/astral-sh/uv)
- Node.js 20+
- Docker (optional)

## Local Development

```bash
# 1. Create venv and build Rust extension (first build ~2–3 min)
uv venv && source .venv/bin/activate
uv pip install scikit-build-core cmake ninja
uv pip install -e . --no-build-isolation
uv pip install fastapi uvicorn yfinance httpx pytest

# 2. Run tests
pytest tests/ -v

# 3. Start FastAPI
uvicorn backtester.api:app --reload --port 8000

# 4. In a separate terminal, start React dev server
cd frontend && npm install && npm run dev
# Open http://localhost:5173
```

## Docker

```bash
# Production build (React compiled, served by FastAPI)
docker compose up api

# Open http://localhost:8000
```

## Writing a Strategy

```python
from backtester.decorators import strategy
from backtester.indicators import sma

@strategy(symbol="AAPL", start="2020-01-01", end="2023-12-31", capital=100_000)
def my_strategy(bars):
    fast = sma(bars, 10)
    slow = sma(bars, 50)
    return [
        1.0 if (f == f and s == s and f > s) else
        -1.0 if (f == f and s == s and f < s) else
        0.0
        for f, s in zip(fast, slow)
    ]

result = my_strategy()
print(f"Return: {result.metrics.total_return:.2%}, Sharpe: {result.metrics.sharpe:.2f}")
```

Register it with the API by placing it in `strategies/examples.py` or setting `STRATEGY_MODULE` env var.
```

- [ ] **Step 4: Verify Docker build**

```bash
docker build -t quant-research-tool .
```

Expected: successful multi-stage build. Fix any path issues before proceeding.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile docker-compose.yml README.md
git commit -m "feat: Dockerfile (3-stage), docker-compose, README with dev setup"
```

---

## Self-Review Notes

**Spec coverage check:**
- ✅ Rust types, indicators, portfolio, execution, metrics, backtest runner
- ✅ PyO3 surface: `BacktestEngine`, `Bar`, `sma/ema/rsi`, `BacktestResult`, `MetricsPy`
- ✅ Python `@strategy` + `@indicator` decorators with `yfinance` data sourcing
- ✅ FastAPI: `/run`, `/strategies`, `/health` + static file serving
- ✅ React: `RunForm`, `MetricsCards`, `EquityCurve`, `App.tsx`, dark CSS
- ✅ CMake + Corrosion + scikit-build-core + uv
- ✅ 3-stage Dockerfile + docker-compose
- ✅ `.gitignore`, `README.md`
- ✅ Tests: `test_engine.py`, `test_decorators.py`, `test_api.py`

**Type consistency check:**
- `BacktestEngine.run(bars, signals, symbol)` — consistent across Task 6 (lib.rs) and Task 7 (conftest/tests)
- `result.equity_curve` → `Vec<EquityPoint>` in Rust, `list[dict]` in API JSON, `EquityPoint[]` in TypeScript — ✅
- `result.metrics.total_return` / `sharpe` / `sortino` / `max_drawdown` / `win_rate` / `num_trades` — consistent across Rust `MetricsPy`, Python API serialization, and TypeScript `Metrics` type ✅
- `@indicator` cache key `(id(bars), fn.__name__, args)` — consistent in Task 9 (decorators.py) and test ✅
