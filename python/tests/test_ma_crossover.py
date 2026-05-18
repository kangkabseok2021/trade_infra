import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))

from strategies.ma_crossover import compute_ema, detect_cross


def test_ema_moves_toward_new_value():
    # alpha = 2/(5+1) ≈ 0.333; ema = 0.333*60 + 0.667*40 ≈ 46.67
    result = compute_ema(lmp=60.0, prev_ema=40.0, period=5)
    assert abs(result - 46.667) < 0.01


def test_ema_unchanged_at_same_value():
    result = compute_ema(lmp=45.0, prev_ema=45.0, period=5)
    assert result == 45.0


def test_buy_cross_fast_above_slow():
    # fast was below slow, now above → BUY
    result = detect_cross(fast_prev=44.0, fast_curr=46.0, slow_prev=45.0, slow_curr=45.0)
    assert result == 'BUY'


def test_sell_cross_fast_below_slow():
    # fast was above slow, now below → SELL
    result = detect_cross(fast_prev=46.0, fast_curr=44.0, slow_prev=45.0, slow_curr=45.0)
    assert result == 'SELL'


def test_no_cross_fast_stays_above():
    result = detect_cross(fast_prev=47.0, fast_curr=46.0, slow_prev=45.0, slow_curr=45.5)
    assert result is None


def test_no_cross_fast_stays_below():
    result = detect_cross(fast_prev=43.0, fast_curr=44.0, slow_prev=45.0, slow_curr=45.0)
    assert result is None


def test_no_cross_equal_values():
    # fast == slow on both sides → no cross
    result = detect_cross(fast_prev=45.0, fast_curr=45.0, slow_prev=45.0, slow_curr=45.0)
    assert result is None
