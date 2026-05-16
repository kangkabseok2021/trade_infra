package listener

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/evaluator"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

type TickPayload struct {
	Node string  `json:"node"`
	LMP  float64 `json:"lmp"`
}

type Listener struct {
	store    *order.Store
	db       *sql.DB
	dbURL    string
	FillsOut chan<- int64
}

func New(store *order.Store, db *sql.DB, dbURL string, fills chan<- int64) *Listener {
	return &Listener{store: store, db: db, dbURL: dbURL, FillsOut: fills}
}

func (l *Listener) Run() {
	onErr := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("listener error: %v", err)
		}
	}
	pl := pq.NewListener(l.dbURL, 10*time.Second, time.Minute, onErr)
	if err := pl.Listen("price_ticks"); err != nil {
		log.Fatalf("LISTEN price_ticks: %v", err)
	}
	log.Println("order-svc: listening on price_ticks")
	for n := range pl.Notify {
		if n == nil {
			continue
		}
		var tick TickPayload
		if err := json.Unmarshal([]byte(n.Extra), &tick); err != nil {
			log.Printf("bad payload: %v", err)
			continue
		}
		l.evaluate(tick)
	}
}

func (l *Listener) evaluate(tick TickPayload) {
	orders, err := l.store.ListPendingByNode(tick.Node)
	if err != nil {
		log.Printf("list pending: %v", err)
		return
	}
	for _, o := range orders {
		if evaluator.ShouldFill(o, tick.LMP) {
			fa := tick.LMP
			if err := l.store.UpdateStatus(o.ID, order.StatusFilled, &fa); err != nil {
				log.Printf("update order %d: %v", o.ID, err)
				continue
			}
			l.notify(o.ID, tick.Node, tick.LMP, o.QuantityMW)
			if l.FillsOut != nil {
				l.FillsOut <- o.ID
			}
		}
	}
}

func (l *Listener) notify(orderID int64, node string, filledLMP, qty float64) {
	payload := fmt.Sprintf(`{"order_id":%d,"node":%q,"filled_lmp":%f,"quantity_mw":%f}`,
		orderID, node, filledLMP, qty)
	l.db.Exec("SELECT pg_notify('order_updates', $1)", payload)
}
