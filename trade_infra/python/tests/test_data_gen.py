import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
from data_gen import generate_price_ticks

def test_returns_correct_count():
    ticks = generate_price_ticks("HB_NORTH", base_lmp=45.0, volatility=5.0, n=10, seed=42)
    assert len(ticks) == 10

def test_tick_has_required_fields():
    ticks = generate_price_ticks("HB_NORTH", base_lmp=45.0, volatility=5.0, n=1, seed=42)
    assert {"node", "lmp", "load_mw"}.issubset(ticks[0].keys())
    assert ticks[0]["node"] == "HB_NORTH"

def test_lmp_stays_in_range():
    ticks = generate_price_ticks("HB_NORTH", base_lmp=45.0, volatility=5.0, n=200, seed=42)
    assert all(0 < t["lmp"] < 500 for t in ticks)

def test_deterministic_with_same_seed():
    t1 = generate_price_ticks("HB_NORTH", 45.0, 5.0, 5, seed=42)
    t2 = generate_price_ticks("HB_NORTH", 45.0, 5.0, 5, seed=42)
    assert [t["lmp"] for t in t1] == [t["lmp"] for t in t2]
