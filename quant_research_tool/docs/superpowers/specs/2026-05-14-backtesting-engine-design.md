# Backtesting Engine — Design Spec
_Date: 2026-05-14_

## Overview

A high-speed hybrid backtesting engine with a Rust core, Python decorator API via PyO3, and a React performance analytics dashboard. Targets equity/ETF daily OHLCV data. Analysts define strategies in Python using a `@strategy` decorator; all heavy computation runs in Rust.

---

## 1. Repository Layout

```
quant_research_tool/
├── src/                        # Rust crate (core engine)
│   ├── lib.rs                  # PyO3 module entry point
│   ├── types.rs                # Bar, Order, Trade, Signal structs
│   ├── indicators.rs           # Vectorized: SMA, EMA, RSI
│   ├── execution.rs            # Event-driven order/fill simulation
│   ├── portfolio.rs            # Position & cash tracking
│   ├── backtest.rs             # Hybrid runner (vectorized → event-driven)
│   └── metrics.rs              # Sharpe, Sortino, drawdown, win rate
├── backtester/                 # Python package
│   ├── __init__.py
│   ├── decorators.py           # @strategy, @indicator
│   └── api.py                  # FastAPI app
├── frontend/                   # Vite + React + TypeScript
│   ├── src/
│   │   ├── App.tsx
│   │   ├── components/
│   │   │   ├── EquityCurve.tsx
│   │   │   ├── MetricsCards.tsx
│   │   │   └── RunForm.tsx
│   │   └── main.tsx
│   └── package.json
├── tests/                      # Python pytest suite
├── CMakeLists.txt              # Corrosion-based Rust → .so build
├── pyproject.toml              # scikit-build-core backend
├── Cargo.toml                  # Rust crate manifest
├── Dockerfile                  # 3-stage multi-arch build
├── docker-compose.yml          # Dev + prod orchestration
├── .gitignore
└── README.md
```

---

## 2. Rust Core

### 2.1 Types (`src/types.rs`)

```rust
pub struct Bar { pub date: i64, pub open: f64, pub high: f64, pub low: f64, pub close: f64, pub volume: f64 }
pub enum Side { Buy, Sell }
pub struct Order { pub date: i64, pub symbol: String, pub qty: f64, pub side: Side }
pub struct Trade { pub entry_date: i64, pub exit_date: i64, pub symbol: String, pub pnl: f64, pub pnl_pct: f64 }
pub struct Signal(pub f64); // +1.0 buy, -1.0 sell, 0.0 flat
```

### 2.2 Hybrid Runner (`src/backtest.rs`)

Two sequential phases per backtest run:

1. **Vectorized phase** — receives `Vec<Bar>` + strategy signal array `Vec<f64>`. No simulation, pure array computation. Lookahead-safe: signals at index `i` are based only on bars `0..=i`.
2. **Event-driven phase** — replays bars in chronological order. On each bar, checks for signal change → emits `Order` → `execution.rs` fills at next bar's open price → `portfolio.rs` updates positions and equity.

### 2.3 Execution Engine (`src/execution.rs`)

- Fill price: next-bar open (eliminates lookahead bias)
- Slippage: configurable basis points applied to fill price
- Commission: flat per-trade fee in dollars
- v1 scope: no partial fills, no short selling, one position per symbol at a time

### 2.4 Metrics (`src/metrics.rs`)

Computed from the equity curve `Vec<f64>`:
- Annualized return
- Sharpe ratio (annualized, risk-free rate configurable, default 0.0)
- Sortino ratio
- Max drawdown (magnitude + duration in bars)
- Win rate
- Average win / average loss ratio
- Number of trades

### 2.5 PyO3 Surface (`src/lib.rs`)

```rust
#[pyclass] pub struct BacktestEngine { /* config */ }

#[pymethods]
impl BacktestEngine {
    #[new]
    pub fn new(capital: f64, slippage_bps: f64, commission: f64) -> Self { ... }
    pub fn run(&self, bars: Vec<BarPy>, signals: Vec<f64>) -> BacktestResult { ... }
}

#[pyclass] pub struct BacktestResult {
    pub equity_curve: Vec<EquityPoint>,  // { date: i64, value: f64 }
    pub trades: Vec<TradePy>,
    pub metrics: MetricsPy,
}
```

All structs exposed to Python are annotated `#[pyclass]` with `#[pyo3(get)]` fields.

---

## 3. Python Layer

### 3.1 Decorator API (`backtester/decorators.py`)

```python
@strategy(symbol="AAPL", start="2020-01-01", end="2023-12-31", capital=100_000)
def sma_crossover(bars: list[Bar]) -> list[float]:
    fast = sma(bars, 10)
    slow = sma(bars, 50)
    return [1.0 if f > s else -1.0 if f < s else 0.0 for f, s in zip(fast, slow)]

# Calling sma_crossover() returns a BacktestResult
result = sma_crossover()
```

**`@strategy` behavior:**
1. On call, fetches OHLCV data via `yfinance` for the configured symbol/date range
2. Converts to `List[BarPy]`
3. Calls the decorated function to get `List[float]` signals
4. Instantiates `BacktestEngine(capital, slippage_bps, commission)` and calls `.run()`
5. Returns `BacktestResult`
6. Self-registers the strategy name in a module-level dict for API lookup

**`@indicator` behavior:**  
Wraps a pure array function with a manual cache dict keyed on `(id(bars), *args)` — uses object identity of the bars list as the key so `List[Bar]` doesn't need to be hashable. Cache is scoped to a single backtest run (cleared between runs).

### 3.2 FastAPI (`backtester/api.py`)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/run` | `{ symbol, start, end, capital, strategy_name }` → `BacktestResult` JSON |
| `GET` | `/strategies` | List of registered strategy names |
| `GET` | `/health` | `{ status: "ok" }` |

Strategies are looked up by name from the self-registration dict. The API imports all strategy modules at startup (configurable via env var `STRATEGY_MODULE`).

**Data sourcing:** `yfinance` in v1. No caching between runs.

---

## 4. React Frontend

### 4.1 Stack

- Vite + React 18 + TypeScript
- Recharts for charting
- No UI component library — custom CSS

### 4.2 Layout

```
┌─────────────────────────────────────────────────┐
│  RunForm: [Strategy ▾] [Symbol] [Start] [End]   │
│           [Capital $]                [Run ▶]    │
├─────────────────────────────────────────────────┤
│  MetricsCards:                                  │
│  [ Total Return ] [ Sharpe ] [ Sortino ]        │
│  [ Max Drawdown ] [ Win Rate ] [ # Trades ]     │
├─────────────────────────────────────────────────┤
│  EquityCurve: area chart                        │
│    x-axis: date  y-axis: portfolio value        │
│    equity series: green fill                    │
│    drawdown series: red, negative overlay       │
└─────────────────────────────────────────────────┘
```

### 4.3 Components

- **`RunForm`** — controlled inputs, calls `POST /run`, manages loading/error state, populates strategy dropdown from `GET /strategies`
- **`MetricsCards`** — six stat tiles from `BacktestResult.metrics`
- **`EquityCurve`** — Recharts `AreaChart`, two series: equity (green) and drawdown (red, negative axis)
- **`App.tsx`** — owns `result: BacktestResult | null`, composes all three components

### 4.4 API Contract (TypeScript)

```ts
type BacktestResult = {
  equity_curve: { date: string; value: number }[];
  metrics: {
    total_return: number;
    sharpe: number;
    sortino: number;
    max_drawdown: number;
    win_rate: number;
    num_trades: number;
  };
  trades: {
    entry_date: string;
    exit_date: string;
    symbol: string;
    pnl: number;
    pnl_pct: number;
  }[];
};
```

### 4.5 Dev vs Production

- **Dev:** Vite dev server (`localhost:5173`) proxies `/run`, `/strategies`, `/health` to FastAPI on `localhost:8000`
- **Production (Docker):** `vite build` outputs to `frontend/dist/`; FastAPI mounts `dist/` as static files — single container, no nginx

---

## 5. Build System

### 5.1 CMakeLists.txt (Corrosion)

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
target_link_libraries(quant_engine INTERFACE Python::Module)
install(TARGETS quant_engine LIBRARY DESTINATION backtester)
```

### 5.2 pyproject.toml

```toml
[build-system]
requires = ["scikit-build-core", "cmake"]
build-backend = "scikit_build_core.build"

[project]
name = "quant-research-tool"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = ["fastapi", "uvicorn", "yfinance"]

[tool.scikit-build]
cmake.version = ">=3.15"
ninja.version = ">=1.10"
```

Note: `pyo3` is declared as a Rust dependency in `Cargo.toml`, not in `pyproject.toml`. scikit-build-core drives CMake, CMake drives Corrosion, Corrosion drives Cargo — the Python build system has no direct knowledge of PyO3.

### 5.3 Dockerfile (3-stage)

```
Stage 1: rust-builder
  - FROM rust:1.78-slim + cmake + ninja + python dev headers
  - COPY . && RUN uv pip install -e . (triggers scikit-build-core → CMake → Corrosion → cargo build)
  - Output: quant_engine.cpython-*.so

Stage 2: node-builder
  - FROM node:20-slim
  - COPY frontend/ && RUN npm ci && npm run build
  - Output: frontend/dist/

Stage 3: final
  - FROM python:3.12-slim
  - COPY --from=rust-builder .so into backtester/
  - COPY --from=node-builder dist/ into backtester/static/
  - COPY backtester/ pyproject.toml
  - RUN uv pip install --no-build-isolation .
  - CMD uvicorn backtester.api:app --host 0.0.0.0 --port 8000
```

### 5.4 docker-compose.yml

```yaml
services:
  api:         # FastAPI + Rust .so, port 8000, --reload in dev
  frontend:    # Vite dev server, port 5173, proxies to api:8000 (dev only)
```

Production: `docker compose up api` only (serves static React from FastAPI).

---

## 6. Testing

- `tests/test_engine.py` — pytest tests calling `BacktestEngine` directly via PyO3
- `tests/test_api.py` — FastAPI `TestClient` tests for all three endpoints
- `tests/test_decorators.py` — tests `@strategy` and `@indicator` with mock OHLCV data
- No Rust unit tests in v1 (covered via Python integration tests)

---

## 7. Git & Tooling

- `.gitignore`: `target/`, `*.so`, `node_modules/`, `.venv/`, `__pycache__/`, `.superpowers/`
- `README.md`: prerequisites (Rust, CMake, uv, Node), local dev setup, Docker instructions
- `git init` at project root; initial commit includes scaffold + this spec
