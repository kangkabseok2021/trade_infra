import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.mean_reversion import compute_signal


def test_no_signal_during_warmup():
    # Fewer ticks than window → still warming up
    result = compute_signal([45.0] * 5, window=20, threshold=1.0)
    assert result is None


def test_buy_signal_below_band():
    # 19 ticks at 45.0, last tick at 30.0 — far below mean
    # mean≈44.25, std≈3.36 → lower band≈40.9; 30.0 < 40.9 → BUY
    lmps = [45.0] * 19 + [30.0]
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result == 'BUY'


def test_sell_signal_above_band():
    # 19 ticks at 45.0, last tick at 60.0 — far above mean
    # mean≈45.75, std≈3.35 → upper band≈49.1; 60.0 > 49.1 → SELL
    lmps = [45.0] * 19 + [60.0]
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result == 'SELL'


def test_no_signal_within_band():
    # Alternating 44/46: mean=45, std≈1.03 → band=[43.97, 46.03]; last=46.0 inside
    lmps = [44.0, 46.0] * 10
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result is None


def test_no_signal_flat_prices():
    # All same → std=0 → no signal (avoids spurious fires)
    result = compute_signal([45.0] * 20, window=20, threshold=1.0)
    assert result is None


def test_only_last_window_used():
    # First 20 values are noise; last 20 are flat at 45.0 → no signal
    lmps = [1.0] * 20 + [45.0] * 20
    result = compute_signal(lmps, window=20, threshold=1.0)
    assert result is None
