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
