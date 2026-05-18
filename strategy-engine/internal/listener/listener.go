package listener

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/gate"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/submitter"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/metrics"
)

type Listener struct {
	dbURL     string
	db        *sql.DB
	store     *signal.Store
	gate      *gate.Gate
	submitter *submitter.Submitter
}

func New(dbURL string, db *sql.DB, store *signal.Store, g *gate.Gate, sub *submitter.Submitter) *Listener {
	return &Listener{dbURL: dbURL, db: db, store: store, gate: g, submitter: sub}
}

func (l *Listener) Run() {
	onErr := func(_ pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("strategy-engine listener error: %v", err)
		}
	}
	pl := pq.NewListener(l.dbURL, 10*time.Second, time.Minute, onErr)
	if err := pl.Listen("signals"); err != nil {
		log.Fatalf("LISTEN signals: %v", err)
	}
	log.Println("strategy-engine: listening on signals")
	for n := range pl.Notify {
		if n == nil {
			continue
		}
		id, err := strconv.ParseInt(n.Extra, 10, 64)
		if err != nil {
			log.Printf("bad signal notify payload: %q", n.Extra)
			continue
		}
		l.process(id)
	}
}

func (l *Listener) process(signalID int64) {
	sig, err := l.store.GetByID(signalID)
	if err != nil || sig == nil {
		log.Printf("get signal %d: %v", signalID, err)
		return
	}

	metrics.SignalsReceived.WithLabelValues(sig.Strategy, sig.Node).Inc()

	allowed, reason := l.gate.Check(sig.Strategy, sig.Node, sig.QuantityMW)
	if !allowed {
		claimed, err := l.store.ClaimPending(signalID, signal.StatusSkipped, &reason)
		if err != nil {
			log.Printf("signal %d skip-claim error: %v", signalID, err)
			return
		}
		if !claimed {
			log.Printf("signal %d already claimed, skip discarded", signalID)
			return
		}
		metrics.SignalsSkipped.WithLabelValues(sig.Strategy, sig.Node, reason).Inc()
		log.Printf("signal %d skipped: %s", signalID, reason)
		return
	}

	claimed, err := l.store.ClaimPending(signalID, signal.StatusSubmitted, nil)
	if err != nil || !claimed {
		log.Printf("signal %d already claimed or error: %v", signalID, err)
		return
	}

	orderID, err := l.submitter.Submit(sig)
	if err != nil {
		log.Printf("signal %d submit failed: %v", signalID, err)
		metrics.SignalsSkipped.WithLabelValues(sig.Strategy, sig.Node, "order_error").Inc()
		return
	}
	if err := l.store.SetOrderID(signalID, orderID); err != nil {
		log.Printf("signal %d: order %d submitted but failed to record order_id: %v", signalID, orderID, err)
	}
	metrics.SignalsSubmitted.WithLabelValues(sig.Strategy, sig.Node).Inc()
	log.Printf("signal %d submitted as order %d (%s %s %.1fMW @ %.4f)",
		signalID, orderID, sig.Strategy, sig.Side, sig.QuantityMW, sig.LimitPrice)
}
