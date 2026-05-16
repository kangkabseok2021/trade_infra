package evaluator

import "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"

// ShouldFill returns true when the current market LMP meets the order's limit price.
// BUY fills when lmp <= limit (we can buy cheaper or at limit).
// SELL fills when lmp >= limit (we can sell at or above our floor).
func ShouldFill(o *order.Order, currentLMP float64) bool {
	switch o.Side {
	case order.SideBuy:
		return currentLMP <= o.LimitPrice
	case order.SideSell:
		return currentLMP >= o.LimitPrice
	default:
		return false
	}
}
