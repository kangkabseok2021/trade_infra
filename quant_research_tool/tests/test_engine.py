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
