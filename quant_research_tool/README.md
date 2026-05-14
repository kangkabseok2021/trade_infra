# quant_research_tool

High-speed backtesting engine: Rust core + PyO3 Python bindings + React dashboard.

## Architecture

- **Rust core** (`src/`): SMA/EMA/RSI indicators, hybrid backtest runner (vectorized signals → event-driven fills), Sharpe/Sortino/drawdown metrics
- **PyO3 bindings** (`src/lib.rs`): Exposes `BacktestEngine`, `Bar`, `sma/ema/rsi` to Python
- **Build system**: CMake + Corrosion (integrates Cargo into CMake), scikit-build-core Python backend
- **Python API** (`backtester/`): `@strategy` / `@indicator` decorators, FastAPI backend
- **Frontend** (`frontend/`): Vite + React + Recharts — equity curve and metrics dashboard

## Prerequisites

- [Rust](https://rustup.rs/) 1.78+ with Python 3.12 (PyO3 0.21 requires Python ≤ 3.12)
- CMake 3.15+
- [uv](https://github.com/astral-sh/uv)
- Node.js 20+
- Docker (optional)

## Local Development

```bash
# 1. Create venv with Python 3.12 and build Rust extension (first build ~2–3 min)
uv venv --python 3.12
source .venv/bin/activate
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

Place strategies in `strategies/examples.py` or set `STRATEGY_MODULE` env var to point to your module.

## Project Structure

```
quant_research_tool/
├── src/          # Rust engine (types, indicators, portfolio, execution, metrics, backtest)
├── backtester/   # Python package (decorators, indicators, FastAPI)
├── frontend/     # Vite + React dashboard
├── strategies/   # Example strategies
└── tests/        # Python integration tests
```
