package evaluator_test

import (
	"testing"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/evaluator"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func makeOrder(side order.Side, limitPrice float64) *order.Order {
	return &order.Order{ID: 1, Side: side, QuantityMW: 10, LimitPrice: limitPrice, Status: order.StatusPending}
}

func TestShouldFill_BuyBelowLimit(t *testing.T) {
	o := makeOrder(order.SideBuy, 50.0)
	if !evaluator.ShouldFill(o, 48.0) {
		t.Error("BUY at 48 should fill with limit 50")
	}
}

func TestShouldFill_BuyAboveLimit(t *testing.T) {
	o := makeOrder(order.SideBuy, 50.0)
	if evaluator.ShouldFill(o, 52.0) {
		t.Error("BUY at 52 should NOT fill with limit 50")
	}
}

func TestShouldFill_SellAboveLimit(t *testing.T) {
	o := makeOrder(order.SideSell, 40.0)
	if !evaluator.ShouldFill(o, 42.0) {
		t.Error("SELL at 42 should fill with limit 40")
	}
}

func TestShouldFill_SellBelowLimit(t *testing.T) {
	o := makeOrder(order.SideSell, 40.0)
	if evaluator.ShouldFill(o, 38.0) {
		t.Error("SELL at 38 should NOT fill with limit 40")
	}
}

func TestShouldFill_AtExactLimit(t *testing.T) {
	if !evaluator.ShouldFill(makeOrder(order.SideBuy, 50.0), 50.0) {
		t.Error("BUY at exactly limit should fill")
	}
	if !evaluator.ShouldFill(makeOrder(order.SideSell, 40.0), 40.0) {
		t.Error("SELL at exactly limit should fill")
	}
}
