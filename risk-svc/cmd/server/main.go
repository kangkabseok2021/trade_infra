package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/handler"
	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/listener"
	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
)

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	dbURL := getenv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/trade_infra")
	apiAddr := getenv("API_ADDR", ":8081")
	metricsAddr := getenv("METRICS_ADDR", ":9103")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	store := risk.NewStore(db)
	l := listener.New(store, db, dbURL)
	go l.Run()

	go func() {
		h := handler.New(store)
		mux := http.NewServeMux()
		mux.Handle("/risk/", h)
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("ok"))
		})
		log.Printf("risk-svc API on %s", apiAddr)
		log.Fatal(http.ListenAndServe(apiAddr, mux))
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	log.Printf("risk-svc metrics on %s", metricsAddr)
	log.Fatal(http.ListenAndServe(metricsAddr, mux))
}
