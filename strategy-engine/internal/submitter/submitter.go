package submitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
)

type Submitter struct {
	orderSvcURL string
	client      *http.Client
}

func New(orderSvcURL string) *Submitter {
	return &Submitter{orderSvcURL: orderSvcURL, client: &http.Client{}}
}

type orderRequest struct {
	Node       string  `json:"node"`
	Side       string  `json:"side"`
	QuantityMW float64 `json:"quantity_mw"`
	LimitPrice float64 `json:"limit_price"`
}

type orderResponse struct {
	ID int64 `json:"id"`
}

func (s *Submitter) Submit(sig *signal.Signal) (int64, error) {
	body, _ := json.Marshal(orderRequest{
		Node:       sig.Node,
		Side:       sig.Side,
		QuantityMW: sig.QuantityMW,
		LimitPrice: sig.LimitPrice,
	})
	resp, err := s.client.Post(s.orderSvcURL+"/orders", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("order-svc returned %d", resp.StatusCode)
	}
	var out orderResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.ID, nil
}
