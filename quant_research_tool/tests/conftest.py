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
