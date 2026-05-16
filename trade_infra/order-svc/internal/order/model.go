package order

import "time"

type Side   string
type Status string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

const (
	StatusPending   Status = "PENDING"
	StatusFilled    Status = "FILLED"
	StatusRejected  Status = "REJECTED"
	StatusCancelled Status = "CANCELLED"
)

type Order struct {
	ID         int64     `json:"id"`
	Node       string    `json:"node"`
	Side       Side      `json:"side"`
	QuantityMW float64   `json:"quantity_mw"`
	LimitPrice float64   `json:"limit_price"`
	Status     Status    `json:"status"`
	FilledAt   *float64  `json:"filled_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CreateOrderRequest struct {
	Node       string  `json:"node"`
	Side       Side    `json:"side"`
	QuantityMW float64 `json:"quantity_mw"`
	LimitPrice float64 `json:"limit_price"`
}
