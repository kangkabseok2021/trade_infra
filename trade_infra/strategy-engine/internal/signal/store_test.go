package signal_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/strategy-engine/internal/signal"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test"
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM signals"); db.Close() })
	return db
}

func insertPending(t *testing.T, s *signal.Store) *signal.Signal {
	t.Helper()
	sig, err := s.Insert("mean_reversion", "HB_NORTH", "BUY", 5.0, 45.0)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return sig
}

func TestStore_Insert(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig, err := s.Insert("mean_reversion", "HB_NORTH", "BUY", 5.0, 45.0)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if sig.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if sig.Status != signal.StatusPending {
		t.Errorf("want PENDING got %s", sig.Status)
	}
}

func TestStore_ClaimPending_Success(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	claimed, err := s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Error("expected claim to succeed")
	}
	got, _ := s.GetByID(sig.ID)
	if got.Status != signal.StatusSubmitted {
		t.Errorf("want SUBMITTED got %s", got.Status)
	}
}

func TestStore_ClaimPending_Idempotent(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	// second claim should return false (already claimed)
	claimed, err := s.ClaimPending(sig.ID, signal.StatusSkipped, nil)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Error("second claim should return false")
	}
}

func TestStore_SetOrderID(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	if err := s.SetOrderID(sig.ID, 42); err != nil {
		t.Fatalf("set order id: %v", err)
	}
	got, _ := s.GetByID(sig.ID)
	if got.OrderID == nil || *got.OrderID != 42 {
		t.Errorf("want order_id=42 got %v", got.OrderID)
	}
}

func TestStore_LatestSubmitted_None(t *testing.T) {
	s := signal.NewStore(testDB(t))
	ts, err := s.LatestSubmitted("mean_reversion", "HB_NORTH")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ts != nil {
		t.Error("expected nil when no submitted signals")
	}
}

func TestStore_LatestSubmitted_Returns(t *testing.T) {
	s := signal.NewStore(testDB(t))
	sig := insertPending(t, s)
	s.ClaimPending(sig.ID, signal.StatusSubmitted, nil)
	ts, err := s.LatestSubmitted("mean_reversion", "HB_NORTH")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ts == nil {
		t.Error("expected non-nil timestamp after submission")
	}
	if time.Since(*ts) > 5*time.Second {
		t.Error("timestamp should be recent")
	}
}
