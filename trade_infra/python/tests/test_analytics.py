import sys, os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
from analytics import format_pnl_report, format_position_summary

def test_pnl_report_positive():
    result = format_pnl_report("HB_NORTH", mtm_pnl=125.50, net_exposure_mw=10.0, limit_headroom=40.0)
    assert "HB_NORTH" in result
    assert "125.50" in result
    assert "WITHIN LIMITS" in result

def test_pnl_report_breach():
    result = format_pnl_report("HB_SOUTH", mtm_pnl=-50.0, net_exposure_mw=55.0, limit_headroom=-5.0)
    assert "BREACH" in result

def test_position_summary_long():
    result = format_position_summary("HB_NORTH", net_mw=10.0, avg_price=45.0)
    assert "LONG" in result
    assert "10.0" in result

def test_position_summary_short():
    result = format_position_summary("HB_NORTH", net_mw=-5.0, avg_price=42.0)
    assert "SHORT" in result

def test_position_summary_flat():
    result = format_position_summary("HB_NORTH", net_mw=0.0, avg_price=None)
    assert "FLAT" in result
