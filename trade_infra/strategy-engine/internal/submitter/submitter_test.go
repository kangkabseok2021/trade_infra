package submitter_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/submitter"
)

func TestSubmitter_Submit_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 99})
	}))
	defer srv.Close()

	sub := submitter.New(srv.URL)
	sig := &signal.Signal{
		ID: 1, Node: "HB_NORTH", Side: "BUY",
		QuantityMW: 5.0, LimitPrice: 45.0,
	}
	orderID, err := sub.Submit(sig)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if orderID != 99 {
		t.Errorf("want orderID=99 got %d", orderID)
	}
}

func TestSubmitter_Submit_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "db error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	sub := submitter.New(srv.URL)
	sig := &signal.Signal{Node: "HB_NORTH", Side: "BUY", QuantityMW: 5.0, LimitPrice: 45.0}
	_, err := sub.Submit(sig)
	if err == nil {
		t.Error("expected error on 500 response")
	}
}
