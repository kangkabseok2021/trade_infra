package risk_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test"
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM risk_snapshots")
		db.Exec("DELETE FROM positions")
		db.Close()
	})
	return db
}

func TestStore_SaveSnapshot(t *testing.T) {
	s := risk.NewStore(testDB(t))
	snap := &risk.Snapshot{Node: "HB_NORTH", MtmPnl: 125.50, NetExposureMW: 10, LimitHeadroom: 40}
	if err := s.SaveSnapshot(snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	if snap.ID == 0 {
		t.Error("expected ID")
	}
}

func TestStore_LatestSnapshot(t *testing.T) {
	s := risk.NewStore(testDB(t))
	s.SaveSnapshot(&risk.Snapshot{Node: "HB_NORTH", MtmPnl: 100, NetExposureMW: 10, LimitHeadroom: 40})
	s.SaveSnapshot(&risk.Snapshot{Node: "HB_NORTH", MtmPnl: 200, NetExposureMW: 12, LimitHeadroom: 38})
	snap, err := s.LatestSnapshot("HB_NORTH")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if snap.MtmPnl != 200 {
		t.Errorf("want 200 got %f", snap.MtmPnl)
	}
}

func TestStore_UpsertPosition(t *testing.T) {
	s := risk.NewStore(testDB(t))
	avg := 45.0
	if err := s.UpsertPosition("HB_NORTH", 10.0, &avg); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	pos, err := s.GetPosition("HB_NORTH")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if pos.NetMW != 10.0 {
		t.Errorf("want 10 got %f", pos.NetMW)
	}
}
