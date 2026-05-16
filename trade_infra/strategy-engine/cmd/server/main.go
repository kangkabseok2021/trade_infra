package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/gate"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/listener"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/riskstore"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/submitter"
)

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func waitForOrderSvc(url string) {
	for {
		resp, err := http.Get(url + "/health")
		if err == nil && resp.StatusCode == 200 {
			log.Println("order-svc is healthy")
			return
		}
		log.Printf("waiting for order-svc at %s...", url)
		time.Sleep(3 * time.Second)
	}
}

func main() {
	dbURL        := getenv("DATABASE_URL",      "postgresql://postgres:postgres@localhost:5432/trade_infra")
	orderSvcURL  := getenv("ORDER_SVC_URL",     "http://localhost:18080")
	metricsAddr  := getenv("METRICS_ADDR",      ":9104")
	posLimitMW   := mustFloat(getenv("POSITION_LIMIT_MW", "40"))
	cooldownSecs := mustInt(getenv("COOLDOWN_SECS",        "30"))

	waitForOrderSvc(orderSvcURL)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	sigStore  := signal.NewStore(db)
	riskStore := riskstore.NewStore(db)
	g         := gate.New(riskStore, sigStore, posLimitMW, cooldownSecs)
	sub       := submitter.New(orderSvcURL)
	l         := listener.New(dbURL, db, sigStore, g, sub)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("ok"))
		})
		log.Printf("strategy-engine metrics on %s", metricsAddr)
		log.Fatal(http.ListenAndServe(metricsAddr, mux))
	}()

	l.Run()
}

func mustFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Fatalf("invalid float %q: %v", s, err)
	}
	return v
}

func mustInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("invalid int %q: %v", s, err)
	}
	return v
}
