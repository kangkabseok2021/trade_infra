package order

import (
	"database/sql"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Create(req CreateOrderRequest) (*Order, error) {
	var o Order
	err := s.db.QueryRow(`
        INSERT INTO orders (node, side, quantity_mw, limit_price)
        VALUES ($1,$2,$3,$4)
        RETURNING id,node,side,quantity_mw,limit_price,status,filled_at,created_at,updated_at`,
		req.Node, string(req.Side), req.QuantityMW, req.LimitPrice,
	).Scan(&o.ID, &o.Node, &o.Side, &o.QuantityMW, &o.LimitPrice,
		&o.Status, &o.FilledAt, &o.CreatedAt, &o.UpdatedAt)
	return &o, err
}

func (s *Store) GetByID(id int64) (*Order, error) {
	var o Order
	err := s.db.QueryRow(`
        SELECT id,node,side,quantity_mw,limit_price,status,filled_at,created_at,updated_at
        FROM orders WHERE id=$1`, id,
	).Scan(&o.ID, &o.Node, &o.Side, &o.QuantityMW, &o.LimitPrice,
		&o.Status, &o.FilledAt, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

func (s *Store) ListPendingByNode(node string) ([]*Order, error) {
	rows, err := s.db.Query(`
        SELECT id,node,side,quantity_mw,limit_price,status,filled_at,created_at,updated_at
        FROM orders WHERE node=$1 AND status='PENDING' ORDER BY created_at`, node)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.Node, &o.Side, &o.QuantityMW, &o.LimitPrice,
			&o.Status, &o.FilledAt, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &o)
	}
	return out, rows.Err()
}

func (s *Store) UpdateStatus(id int64, status Status, filledAt *float64) error {
	_, err := s.db.Exec(`
        UPDATE orders SET status=$1,filled_at=$2,updated_at=$3 WHERE id=$4`,
		string(status), filledAt, time.Now().UTC(), id)
	return err
}
