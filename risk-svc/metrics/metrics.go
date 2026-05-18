package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SnapshotsSaved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "risk_svc_snapshots_saved_total",
		Help: "Total risk snapshots written",
	})
	LimitBreaches = promauto.NewCounter(prometheus.CounterOpts{
		Name: "risk_svc_limit_breaches_total",
		Help: "Total position limit breaches detected",
	})
)
