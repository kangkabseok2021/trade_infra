package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/handler"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/listener"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	dbURL       := getenv("DATABASE_URL",  "postgresql://postgres:postgres@localhost:5432/trade_infra")
	apiAddr     := getenv("API_ADDR",      ":8080")
	metricsAddr := getenv("METRICS_ADDR",  ":9102")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	store := order.NewStore(db)
	fills := make(chan int64, 100)
	l := listener.New(store, db, dbURL, fills)
	go l.Run()

	// REST API
	go func() {
		h := handler.New(store)
		mux := http.NewServeMux()
		mux.Handle("/orders", h)
		mux.Handle("/orders/", h)
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("ok"))
		})
		log.Printf("order-svc API listening on %s", apiAddr)
		log.Fatal(http.ListenAndServe(apiAddr, mux))
	}()

	// Prometheus metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	log.Printf("order-svc metrics listening on %s", metricsAddr)
	log.Fatal(http.ListenAndServe(metricsAddr, mux))
}
