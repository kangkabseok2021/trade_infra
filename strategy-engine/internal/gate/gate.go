package gate

import "time"

type RiskQuerier interface {
	LatestNetExposure(node string) (float64, error)
}

type SignalQuerier interface {
	LatestSubmitted(strategy, node string) (*time.Time, error)
}

type Gate struct {
	risk         RiskQuerier
	signals      SignalQuerier
	posLimitMW   float64
	cooldownSecs int
}

func New(risk RiskQuerier, signals SignalQuerier, posLimitMW float64, cooldownSecs int) *Gate {
	return &Gate{risk: risk, signals: signals, posLimitMW: posLimitMW, cooldownSecs: cooldownSecs}
}

// Check returns (true, "") if the signal may be submitted, or (false, reason) if blocked.
func (g *Gate) Check(strategy, node string, quantityMW float64) (bool, string) {
	netExp, err := g.risk.LatestNetExposure(node)
	if err != nil {
		return false, "risk_query_error"
	}
	if netExp+quantityMW >= g.posLimitMW {
		return false, "risk_limit"
	}

	latest, err := g.signals.LatestSubmitted(strategy, node)
	if err != nil {
		return false, "signal_query_error"
	}
	if latest != nil {
		if time.Since(*latest).Seconds() < float64(g.cooldownSecs) {
			return false, "cooldown"
		}
	}
	return true, ""
}
