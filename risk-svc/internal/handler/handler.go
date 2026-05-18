package handler

import (
	"encoding/json"
	"net/http"

	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
)

type Handler struct {
	store *risk.Store
	mux   *http.ServeMux
}

func New(store *risk.Store) *Handler {
	h := &Handler{store: store, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /risk/{node}", h.getSnapshot)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) getSnapshot(w http.ResponseWriter, r *http.Request) {
	node := r.PathValue("node")
	snap, err := h.store.LatestSnapshot(node)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if snap == nil {
		http.Error(w, "no snapshot", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}
