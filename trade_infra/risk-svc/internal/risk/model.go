package risk

import "time"

type Snapshot struct {
	ID            int64     `json:"id"`
	Node          string    `json:"node"`
	MtmPnl        float64   `json:"mtm_pnl"`
	NetExposureMW float64   `json:"net_exposure_mw"`
	LimitHeadroom float64   `json:"limit_headroom"`
	SnapshotAt    time.Time `json:"snapshot_at"`
}

type Position struct {
	Node     string   `json:"node"`
	NetMW    float64  `json:"net_mw"`
	AvgPrice *float64 `json:"avg_price,omitempty"`
}
