package order_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test"
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("DELETE FROM orders"); err != nil {
		t.Fatalf("pre-test cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM orders"); db.Close() })
	return db
}

func TestStore_Create(t *testing.T) {
	s := order.NewStore(testDB(t))
	o, err := s.Create(order.CreateOrderRequest{
		Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 10, LimitPrice: 50,
	})
	if err != nil { t.Fatalf("create: %v", err) }
	if o.ID == 0 { t.Error("expected non-zero ID") }
	if o.Status != order.StatusPending { t.Errorf("want PENDING got %s", o.Status) }
}

func TestStore_GetByID(t *testing.T) {
	s := order.NewStore(testDB(t))
	created, err := s.Create(order.CreateOrderRequest{
		Node: "HB_SOUTH", Side: order.SideSell, QuantityMW: 5, LimitPrice: 40,
	})
	if err != nil { t.Fatalf("create: %v", err) }
	got, err := s.GetByID(created.ID)
	if err != nil { t.Fatalf("get: %v", err) }
	if got.Node != "HB_SOUTH" { t.Errorf("want HB_SOUTH got %s", got.Node) }
}

func TestStore_ListPendingByNode(t *testing.T) {
	s := order.NewStore(testDB(t))
	if _, err := s.Create(order.CreateOrderRequest{Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 10, LimitPrice: 50}); err != nil { t.Fatalf("create1: %v", err) }
	if _, err := s.Create(order.CreateOrderRequest{Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 5, LimitPrice: 45}); err != nil { t.Fatalf("create2: %v", err) }
	if _, err := s.Create(order.CreateOrderRequest{Node: "HB_SOUTH", Side: order.SideBuy, QuantityMW: 8, LimitPrice: 42}); err != nil { t.Fatalf("create3: %v", err) }
	orders, err := s.ListPendingByNode("HB_NORTH")
	if err != nil { t.Fatalf("list: %v", err) }
	if len(orders) != 2 { t.Errorf("want 2 got %d", len(orders)) }
}

func TestStore_UpdateStatus(t *testing.T) {
	s := order.NewStore(testDB(t))
	o, err := s.Create(order.CreateOrderRequest{
		Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 10, LimitPrice: 50,
	})
	if err != nil { t.Fatalf("create: %v", err) }
	fa := 48.5
	if err := s.UpdateStatus(o.ID, order.StatusFilled, &fa); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetByID(o.ID)
	if got.Status != order.StatusFilled { t.Errorf("want FILLED got %s", got.Status) }
	if got.FilledAt == nil || *got.FilledAt != 48.5 { t.Errorf("bad filled_at: %v", got.FilledAt) }
}
