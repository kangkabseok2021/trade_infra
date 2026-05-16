package riskcalc_test

import (
	"testing"

	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/riskcalc"
)

func TestMtmPnlLong(t *testing.T) {
	got := riskcalc.CalcMtmPnl(10.0, 40.0, 45.0)
	if got != 50.0 {
		t.Errorf("want 50.0 got %f", got)
	}
}

func TestNetExposureAbsolute(t *testing.T) {
	got := riskcalc.CalcNetExposure(-10.0, 45.0)
	if got != 450.0 {
		t.Errorf("want 450.0 got %f", got)
	}
}

func TestLimitBreach(t *testing.T) {
	if !riskcalc.CheckLimitBreach(15.0, 10.0) {
		t.Error("15 > 10 should breach")
	}
	if riskcalc.CheckLimitBreach(8.0, 10.0) {
		t.Error("8 <= 10 should not breach")
	}
}
