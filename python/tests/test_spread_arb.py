import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.spread_arb import compute_spread_signal


def test_below_window_returns_none():
    # Only 5 spreads, window=20 — still warming up
    result = compute_spread_signal([5.0] * 5, window=20, threshold=1.5)
    assert result is None


def test_flat_spread_returns_none():
    # std=0 → guard against division by zero
    result = compute_spread_signal([3.0] * 20, window=20, threshold=1.5)
    assert result is None


def test_within_band_returns_none():
    # Alternating 0.0/2.0 → mean=1.0, std≈1.03, last z≈0.97 < 1.5
    spreads = [0.0, 2.0] * 10
    result = compute_spread_signal(spreads, window=20, threshold=1.5)
    assert result is None


def test_below_band_returns_buy_sell():
    # 19 spreads at 0.0, last at -20.0
    # mean=-1.0, std≈4.47, z≈-4.25 < -1.5 → BUY node_a, SELL node_b
    spreads = [0.0] * 19 + [-20.0]
    result = compute_spread_signal(spreads, window=20, threshold=1.5)
    assert result == ('BUY', 'SELL')


def test_above_band_returns_sell_buy():
    # 19 spreads at 0.0, last at +20.0
    # mean=1.0, std≈4.47, z≈+4.25 > 1.5 → SELL node_a, BUY node_b
    spreads = [0.0] * 19 + [20.0]
    result = compute_spread_signal(spreads, window=20, threshold=1.5)
    assert result == ('SELL', 'BUY')
