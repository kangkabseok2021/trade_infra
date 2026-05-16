package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SignalsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "strategy_engine_signals_received_total",
		Help: "Total signals received from strategies",
	}, []string{"strategy", "node"})

	SignalsSubmitted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "strategy_engine_signals_submitted_total",
		Help: "Total signals submitted as orders",
	}, []string{"strategy", "node"})

	SignalsSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "strategy_engine_signals_skipped_total",
		Help: "Total signals skipped by gate",
	}, []string{"strategy", "node", "reason"})
)
