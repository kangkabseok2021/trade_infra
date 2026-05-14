from backtester.decorators import strategy
from backtester.indicators import ema, sma


@strategy(symbol="AAPL", start="2020-01-01", end="2023-12-31", capital=100_000)
def sma_crossover(bars):
    """Buy when 10-day SMA crosses above 50-day SMA; sell when it crosses below."""
    fast = sma(bars, 10)
    slow = sma(bars, 50)
    return [
        1.0 if (f == f and s == s and f > s) else
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
