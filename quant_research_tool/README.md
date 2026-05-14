# quant_research_tool

A high-speed hybrid backtesting engine with a Rust core, Python decorator API, and a React analytics dashboard.

The engine runs vectorized signal generation in Python, feeds the signals into an event-driven Rust execution layer, and computes performance metrics entirely in Rust — all exposed to Python via PyO3 bindings built with CMake + Corrosion.

---

## Demo

`sma_crossover` on AAPL (2020–2023, $100k starting capital):

| Total Return | Sharpe | Sortino | Max Drawdown | Win Rate | Trades |
|:---:|:---:|:---:|:---:|:---:|:---:|
| **+102.03%** | 0.97 | 0.79 | 26.75% | 50% | 12 |

---

## Architecture

```
┌─────────────────────────────────────────────┐
│  Python (@strategy decorator)               │
│  yfinance → List[Bar] → signal fn → signals │
└──────────────────┬──────────────────────────┘
                   │ PyO3 (BacktestEngine.run)
┌──────────────────▼──────────────────────────┐
│  Rust core                                  │
│  ┌──────────┐  ┌───────────┐  ┌──────────┐ │
│  │indicators│  │ execution │  │ metrics  │ │
│  │sma/ema/  │  │fill@next  │  │Sharpe    │ │
│  │rsi       │  │bar open   │  │Sortino   │ │
│  └──────────┘  └───────────┘  │drawdown  │ │
│                ┌───────────┐  └──────────┘ │
│                │ portfolio │               │
│                │cash/pos   │               │
│                └───────────┘               │
└──────────────────┬──────────────────────────┘
                   │ BacktestResult (PyO3)
┌──────────────────▼──────────────────────────┐
│  FastAPI  /run  /strategies  /health        │
└──────────────────┬──────────────────────────┘
                   │ JSON
┌──────────────────▼──────────────────────────┐
│  React dashboard                            │
│  equity curve · drawdown · metrics cards   │
└─────────────────────────────────────────────┘
```

**Execution model — hybrid:**
1. **Vectorized phase** — your Python strategy function receives the full `List[Bar]` and returns a `List[float]` signal array (`+1.0` buy, `-1.0` sell, `0.0` flat). No simulation yet.
2. **Event-driven phase** — Rust replays bars chronologically. Signal transitions trigger orders filled at the *next* bar's open (eliminates lookahead bias). Slippage and commission applied per fill.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Core engine | Rust 1.78, PyO3 0.21 |
| Python build | CMake 3.15 + [Corrosion](https://github.com/corrosion-rs/corrosion) + scikit-build-core |
| Package manager | [uv](https://github.com/astral-sh/uv) |
| API | FastAPI + uvicorn |
| Data | yfinance (daily OHLCV) |
| Frontend | Vite + React 18 + TypeScript + Recharts |
| Containers | Docker (3-stage), docker-compose |

---

## Prerequisites

- **Rust** 1.78+ — install via [rustup](https://rustup.rs/)
- **CMake** 3.15+ — `brew install cmake` / `apt install cmake`
- **Python 3.12** — PyO3 0.21 requires Python ≤ 3.12
- **uv** — `curl -LsSf https://astral.sh/uv/install.sh | sh`
- **Node.js** 20+
- **Docker** (optional, for containerised runs)

---

## Quick Start (local)

```bash
# 1. Clone
git clone https://github.com/kangkabseok2021/quant_research_tool
cd quant_research_tool

# 2. Create venv with Python 3.12 and build the Rust extension
#    First build downloads Corrosion and compiles the crate (~2–3 min)
uv venv --python 3.12
source .venv/bin/activate          # Windows: .venv\Scripts\activate
uv pip install scikit-build-core cmake ninja
uv pip install -e . --no-build-isolation
uv pip install fastapi uvicorn yfinance httpx pytest

# 3. Run tests (17 passing)
pytest tests/ -v

# 4. Start backend
uvicorn backtester.api:app --reload --port 8000

# 5. Start frontend (new terminal)
cd frontend && npm install && npm run dev
# → http://localhost:5173
```

---

## Writing a Strategy

Create a function decorated with `@strategy`. It receives a list of `Bar` objects and must return a signal list of the same length.

```python
# strategies/my_strategies.py
from backtester.decorators import strategy
from backtester.indicators import sma, ema, rsi

@strategy(symbol="AAPL", start="2020-01-01", end="2023-12-31", capital=100_000)
def sma_crossover(bars):
    """Buy when 10-day SMA > 50-day SMA; sell when below."""
    fast = sma(bars, 10)
    slow = sma(bars, 50)
    return [
        1.0 if (f == f and s == s and f > s) else   # NaN-safe
        -1.0 if (f == f and s == s and f < s) else
        0.0
        for f, s in zip(fast, slow)
    ]

# Run from Python
result = sma_crossover()
print(f"Return: {result.metrics.total_return:.2%}")
print(f"Sharpe: {result.metrics.sharpe:.2f}")
print(f"Trades: {result.metrics.num_trades}")
```

**Decorator parameters:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `symbol` | required | Ticker (e.g. `"AAPL"`) |
| `start` | required | Start date `"YYYY-MM-DD"` |
| `end` | required | End date `"YYYY-MM-DD"` |
| `capital` | `100_000.0` | Starting capital in dollars |
| `slippage_bps` | `5.0` | Slippage in basis points per fill |
| `commission` | `1.0` | Flat commission in dollars per trade |

**Built-in indicators** (all NaN-padded, call inside `@strategy`):

```python
from backtester.indicators import sma, ema, rsi

sma(bars, period=20)   # simple moving average of close prices
ema(bars, period=20)   # exponential moving average
rsi(bars, period=14)   # Wilder's RSI, values 0–100
```

Repeated calls with the same `bars` and `period` are memoised within a run — no recomputation.

### Registering strategies with the API

The API auto-imports the module named by `STRATEGY_MODULE` (default: `strategies.examples`). Any function decorated with `@strategy` in that module is automatically available via `GET /strategies` and `POST /run`.

```bash
STRATEGY_MODULE=my_package.signals uvicorn backtester.api:app --reload
```

---

## API Reference

| Method | Path | Body | Response |
|--------|------|------|----------|
| `GET` | `/health` | — | `{"status": "ok"}` |
| `GET` | `/strategies` | — | `["sma_crossover", ...]` |
| `POST` | `/run` | `{"strategy_name": "sma_crossover"}` | `BacktestResult` |

**`BacktestResult` shape:**

```json
{
  "equity_curve": [{"date": 1577836800, "value": 100000.0}, ...],
  "metrics": {
    "total_return": 1.0203,
    "annualized_return": 0.2612,
    "sharpe": 0.97,
    "sortino": 0.79,
    "max_drawdown": 0.2675,
    "win_rate": 0.5,
    "num_trades": 12
  },
  "trades": [
    {"entry_date": 1580000000, "exit_date": 1582000000,
     "symbol": "AAPL", "pnl": 1234.56, "pnl_pct": 0.062}
  ]
}
```

Dates are Unix timestamps (seconds). Multiply by 1000 for JavaScript `Date`.

---

## Docker

```bash
# Production — React compiled, served by FastAPI at :8000
docker compose up api
# → http://localhost:8000

# Dev mode — hot-reload FastAPI (run separately: cd frontend && npm run dev)
docker compose --profile dev up api-dev
```

The production image is a 3-stage build:
1. `rust-builder` — installs Rust + CMake, builds the PyO3 `.so`
2. `node-builder` — runs `npm ci && npm run build`
3. `final` — Python 3.12 slim, copies `.so` + React `dist/`, runs uvicorn

---

## Testing

```bash
pytest tests/ -v
```

| Suite | Tests | Covers |
|-------|-------|--------|
| `test_engine.py` | 9 | `BacktestEngine`, `Bar`, `sma/ema/rsi` via PyO3 |
| `test_decorators.py` | 4 | `@strategy` wrapping, self-registration, indicator cache |
| `test_api.py` | 4 | `/health`, `/strategies`, `/run` (success + 404) |

---

## Project Structure

```
quant_research_tool/
├── src/                   # Rust crate
│   ├── lib.rs             # PyO3 module entry point
│   ├── types.rs           # Bar, Trade, EquityPoint structs
│   ├── indicators.rs      # sma(), ema(), rsi()
│   ├── execution.rs       # Fill price + slippage
│   ├── portfolio.rs       # Cash/position tracking, PnL recording
│   ├── backtest.rs        # Hybrid runner (signal array → event loop)
│   └── metrics.rs         # Sharpe, Sortino, drawdown, win rate
├── backtester/            # Python package
│   ├── __init__.py
│   ├── decorators.py      # @strategy, @indicator
│   ├── indicators.py      # sma/ema/rsi wrappers
│   └── api.py             # FastAPI app
├── strategies/
│   └── examples.py        # sma_crossover, ema_momentum
├── frontend/              # Vite + React + TypeScript
│   └── src/
│       ├── App.tsx
│       ├── components/
│       │   ├── RunForm.tsx
│       │   ├── MetricsCards.tsx
│       │   └── EquityCurve.tsx
│       └── types.ts
├── tests/
│   ├── conftest.py
│   ├── test_engine.py
│   ├── test_decorators.py
│   └── test_api.py
├── CMakeLists.txt         # Corrosion-based Rust → .so build
├── Cargo.toml
├── pyproject.toml         # scikit-build-core backend
├── Dockerfile
└── docker-compose.yml
```

---

## Notes

- **Python version**: PyO3 0.21 requires Python ≤ 3.12. Use `uv venv --python 3.12`.
- **macOS linker**: `.cargo/config.toml` adds `-undefined dynamic_lookup` for PyO3 extension-module builds on Apple Silicon and x86_64.
- **yfinance columns**: The decorator normalises yfinance's MultiIndex columns automatically, so single-ticker downloads work with any yfinance version ≥ 0.2.
- **Signal convention**: `+1.0` = buy, `-1.0` = sell, `0.0` = flat. Only transitions trigger orders (rising edge for buy, falling edge for sell).
