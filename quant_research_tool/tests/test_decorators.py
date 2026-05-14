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
