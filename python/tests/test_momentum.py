import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.momentum import compute_momentum_signal


def test_below_window_returns_none():
    # Only 5 lmps, window=20 — need window+1=21 values
    result = compute_momentum_signal([45.0] * 5, window=20, threshold_pct=2.0)
    assert result is None


def test_buy_signal_on_rise():
    # 21 values: first=45.0, last=46.0 → ROC = (46-45)/45*100 = 2.22% > 2.0 → BUY
    lmps = [45.0] * 20 + [46.0]
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result == 'BUY'


def test_sell_signal_on_fall():
    # 21 values: first=45.0, last=43.0 → ROC = (43-45)/45*100 = -4.44% < -2.0 → SELL
    lmps = [45.0] * 20 + [43.0]
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result == 'SELL'


def test_within_band_returns_none():
    # 21 values: first=45.0, last=45.5 → ROC = (45.5-45)/45*100 = 1.11% < 2.0 → None
    lmps = [45.0] * 20 + [45.5]
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result is None


def test_zero_base_returns_none():
    # base price is 0.0 → division by zero guard → None
    lmps = [0.0] + [45.0] * 20
    result = compute_momentum_signal(lmps, window=20, threshold_pct=2.0)
    assert result is None
