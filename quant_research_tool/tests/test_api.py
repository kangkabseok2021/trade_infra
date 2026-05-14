import pandas as pd
from unittest.mock import patch
from fastapi.testclient import TestClient


def _mock_df():
    dates = pd.date_range("2020-01-02", periods=10, freq="B")
    prices = [100.0, 101.0, 102.0, 103.0, 102.0, 101.0, 103.0, 105.0, 104.0, 106.0]
    return pd.DataFrame(
        {"Open": prices, "High": [p + 1 for p in prices],
         "Low": [p - 1 for p in prices], "Close": prices, "Volume": [1000.0] * 10},
        index=dates,
    )


def test_health():
    from backtester.api import app
    client = TestClient(app)
    assert client.get("/health").json() == {"status": "ok"}


def test_strategies_returns_list():
    from backtester.api import app
    client = TestClient(app)
    resp = client.get("/strategies")
    assert resp.status_code == 200
    assert isinstance(resp.json(), list)


def test_run_unknown_returns_404():
    from backtester.api import app
    client = TestClient(app)
    resp = client.post("/run", json={"strategy_name": "does_not_exist"})
    assert resp.status_code == 404


def test_run_registered_strategy():
    with patch("yfinance.download", return_value=_mock_df()):
        from backtester.decorators import strategy, _REGISTRY
        from backtester.api import app

        @strategy(symbol="AAPL", start="2020-01-01", end="2020-01-15", capital=10_000)
        def _api_test_strat(bars):
            return [1.0] * len(bars)

        client = TestClient(app)
        with patch("yfinance.download", return_value=_mock_df()):
            resp = client.post("/run", json={"strategy_name": "_api_test_strat"})

        assert resp.status_code == 200
        body = resp.json()
        assert "equity_curve" in body
        assert "metrics" in body
        assert "trades" in body
        assert isinstance(body["equity_curve"], list)
        assert "total_return" in body["metrics"]
