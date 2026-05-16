package signal

import (
	"database/sql"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Insert(strategy, node, side string, qtyMW, limitPrice float64) (*Signal, error) {
	var sig Signal
	err := s.db.QueryRow(`
		INSERT INTO signals (strategy, node, side, quantity_mw, limit_price)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id,strategy,node,side,quantity_mw,limit_price,status,reason,order_id,created_at`,
		strategy, node, side, qtyMW, limitPrice,
	).Scan(&sig.ID, &sig.Strategy, &sig.Node, &sig.Side,
		&sig.QuantityMW, &sig.LimitPrice, &sig.Status,
		&sig.Reason, &sig.OrderID, &sig.CreatedAt)
	return &sig, err
}

func (s *Store) GetByID(id int64) (*Signal, error) {
	var sig Signal
	err := s.db.QueryRow(`
		SELECT id,strategy,node,side,quantity_mw,limit_price,status,reason,order_id,created_at
		FROM signals WHERE id=$1`, id,
	).Scan(&sig.ID, &sig.Strategy, &sig.Node, &sig.Side,
		&sig.QuantityMW, &sig.LimitPrice, &sig.Status,
		&sig.Reason, &sig.OrderID, &sig.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &sig, err
}

// ClaimPending atomically transitions PENDING → status. Returns false if already claimed.
func (s *Store) ClaimPending(id int64, status Status, reason *string) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE signals SET status=$1, reason=$2 WHERE id=$3 AND status='PENDING'`,
		string(status), reason, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) SetOrderID(id int64, orderID int64) error {
	_, err := s.db.Exec(`UPDATE signals SET order_id=$1 WHERE id=$2`, orderID, id)
	return err
}

func (s *Store) LatestSubmitted(strategy, node string) (*time.Time, error) {
	var ts time.Time
	err := s.db.QueryRow(`
		SELECT created_at FROM signals
		WHERE strategy=$1 AND node=$2 AND status='SUBMITTED'
		ORDER BY created_at DESC LIMIT 1`, strategy, node,
	).Scan(&ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ts, nil
}
