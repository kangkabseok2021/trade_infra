package riskstore

import (
	"database/sql"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// LatestNetExposure returns the most recent net_exposure_mw for the node,
// or 0 if no risk snapshot exists yet.
func (s *Store) LatestNetExposure(node string) (float64, error) {
	var v float64
	err := s.db.QueryRow(`
		SELECT net_exposure_mw FROM risk_snapshots
		WHERE node=$1 ORDER BY snapshot_at DESC LIMIT 1`, node,
	).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
