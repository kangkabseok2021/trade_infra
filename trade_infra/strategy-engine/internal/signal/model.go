package signal

import "time"

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusSubmitted Status = "SUBMITTED"
	StatusSkipped   Status = "SKIPPED"
)

type Signal struct {
	ID         int64     `json:"id"`
	Strategy   string    `json:"strategy"`
	Node       string    `json:"node"`
	Side       string    `json:"side"`
	QuantityMW float64   `json:"quantity_mw"`
	LimitPrice float64   `json:"limit_price"`
	Status     Status    `json:"status"`
	Reason     *string   `json:"reason,omitempty"`
	OrderID    *int64    `json:"order_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
