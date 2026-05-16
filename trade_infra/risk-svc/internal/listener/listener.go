package listener

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/riskcalc"
	"github.com/kangkabseok2021/trade_infra/risk-svc/metrics"
)

const positionLimitMW = 50.0

type FillPayload struct {
	OrderID    int64   `json:"order_id"`
	Node       string  `json:"node"`
	Side       string  `json:"side"`
	FilledLMP  float64 `json:"filled_lmp"`
	QuantityMW float64 `json:"quantity_mw"`
}

type Listener struct {
	store *risk.Store
	db    *sql.DB
	dbURL string
}

func New(store *risk.Store, db *sql.DB, dbURL string) *Listener {
	return &Listener{store: store, db: db, dbURL: dbURL}
}

func (l *Listener) Run() {
	onErr := func(_ pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("risk listener error: %v", err)
		}
	}
	pl := pq.NewListener(l.dbURL, 10*time.Second, time.Minute, onErr)
	if err := pl.Listen("order_updates"); err != nil {
		log.Fatalf("LISTEN order_updates: %v", err)
	}
	log.Println("risk-svc: listening on order_updates")
	for n := range pl.Notify {
		if n == nil {
			continue
		}
		var fill FillPayload
		if err := json.Unmarshal([]byte(n.Extra), &fill); err != nil {
			log.Printf("bad payload: %v", err)
			continue
		}
		l.process(fill)
	}
}

func (l *Listener) process(fill FillPayload) {
	// BUY adds to position; SELL subtracts.
	signedQty := fill.QuantityMW
	if fill.Side == "SELL" {
		signedQty = -fill.QuantityMW
	}

	pos, err := l.store.GetPosition(fill.Node)
	if err != nil {
		log.Printf("get position %s: %v", fill.Node, err)
		return
	}
	netMW := signedQty
	avgPrice := fill.FilledLMP
	if pos != nil && pos.NetMW != 0 && pos.AvgPrice != nil {
		netMW = pos.NetMW + signedQty
		if netMW == 0 {
			avgPrice = 0 // position flattened; cost basis reset
		} else {
			avgPrice = ((*pos.AvgPrice * pos.NetMW) + (fill.FilledLMP * signedQty)) / netMW
		}
	}
	if err := l.store.UpsertPosition(fill.Node, netMW, &avgPrice); err != nil {
		log.Printf("upsert position %s: %v", fill.Node, err)
		return
	}

	mtmPnl := riskcalc.CalcMtmPnl(netMW, avgPrice, fill.FilledLMP)
	netExp := riskcalc.CalcNetExposure(netMW, fill.FilledLMP)
	headroom := positionLimitMW - netExp

	snap := &risk.Snapshot{
		Node: fill.Node, MtmPnl: mtmPnl,
		NetExposureMW: netExp, LimitHeadroom: headroom,
	}
	if err := l.store.SaveSnapshot(snap); err != nil {
		log.Printf("save snapshot: %v", err)
		return
	}
	metrics.SnapshotsSaved.Inc()

	if riskcalc.CheckLimitBreach(netExp, positionLimitMW) {
		log.Printf("LIMIT BREACH: node=%s net_exposure=%.2f limit=%.2f", fill.Node, netExp, positionLimitMW)
		metrics.LimitBreaches.Inc()
	}
}
