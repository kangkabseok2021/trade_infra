package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/handler"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func testHandler(t *testing.T) *handler.Handler {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test"
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM orders"); db.Close() })
	return handler.New(order.NewStore(db))
}

func TestCreateOrder_Returns201(t *testing.T) {
	h := testHandler(t)
	body, _ := json.Marshal(map[string]any{
		"node": "HB_NORTH", "side": "BUY", "quantity_mw": 10.0, "limit_price": 50.0,
	})
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 got %d: %s", w.Code, w.Body)
	}
	var o order.Order
	json.NewDecoder(w.Body).Decode(&o)
	if o.ID == 0 {
		t.Error("expected ID in response")
	}
}

func TestGetOrder_Returns200(t *testing.T) {
	h := testHandler(t)
	body, _ := json.Marshal(map[string]any{
		"node": "HB_SOUTH", "side": "SELL", "quantity_mw": 5.0, "limit_price": 40.0,
	})
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var created order.Order
	json.NewDecoder(w.Body).Decode(&created)

	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%d", created.ID), nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w2.Code)
	}
}

func TestCreateOrder_InvalidSide_Returns400(t *testing.T) {
	h := testHandler(t)
	body, _ := json.Marshal(map[string]any{
		"node": "HB_NORTH", "side": "HOLD", "quantity_mw": 10.0, "limit_price": 50.0,
	})
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 got %d", w.Code)
	}
}
