package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

type Handler struct {
	store *order.Store
	mux   *http.ServeMux
}

func New(store *order.Store) *Handler {
	h := &Handler{store: store, mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /orders", h.createOrder)
	h.mux.HandleFunc("GET /orders/{id}", h.getOrder)
	h.mux.HandleFunc("GET /orders", h.listOrders)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req order.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Side != order.SideBuy && req.Side != order.SideSell {
		http.Error(w, "side must be BUY or SELL", http.StatusBadRequest)
		return
	}
	if req.QuantityMW <= 0 || req.LimitPrice <= 0 {
		http.Error(w, "quantity_mw and limit_price must be positive", http.StatusBadRequest)
		return
	}
	o, err := h.store.Create(req)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	o, err := h.store.GetByID(id)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if o == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get("node")
	if node == "" {
		http.Error(w, "node query param required", http.StatusBadRequest)
		return
	}
	orders, err := h.store.ListPendingByNode(node)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}
