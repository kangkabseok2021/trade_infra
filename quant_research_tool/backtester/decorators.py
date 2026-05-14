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
            # yfinance >= 0.2 returns MultiIndex columns for single-ticker downloads
            if hasattr(df.columns, "levels"):
                df.columns = df.columns.get_level_values(0)
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
