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
