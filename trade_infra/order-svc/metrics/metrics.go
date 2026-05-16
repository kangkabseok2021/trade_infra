package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OrdersCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "order_svc_orders_created_total",
		Help: "Total orders created",
	})
	OrdersFilled = promauto.NewCounter(prometheus.CounterOpts{
		Name: "order_svc_orders_filled_total",
		Help: "Total orders filled",
	})
	EvalLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "order_svc_eval_latency_seconds",
		Help:    "Order evaluation latency",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
	})
)
