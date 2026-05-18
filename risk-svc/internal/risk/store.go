package risk

import "database/sql"

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) SaveSnapshot(snap *Snapshot) error {
	return s.db.QueryRow(`
		INSERT INTO risk_snapshots (node, mtm_pnl, net_exposure_mw, limit_headroom)
		VALUES ($1,$2,$3,$4) RETURNING id,snapshot_at`,
		snap.Node, snap.MtmPnl, snap.NetExposureMW, snap.LimitHeadroom,
	).Scan(&snap.ID, &snap.SnapshotAt)
}

func (s *Store) LatestSnapshot(node string) (*Snapshot, error) {
	var snap Snapshot
	err := s.db.QueryRow(`
		SELECT id,node,mtm_pnl,net_exposure_mw,limit_headroom,snapshot_at
		FROM risk_snapshots WHERE node=$1 ORDER BY snapshot_at DESC LIMIT 1`, node,
	).Scan(&snap.ID, &snap.Node, &snap.MtmPnl, &snap.NetExposureMW, &snap.LimitHeadroom, &snap.SnapshotAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &snap, err
}

func (s *Store) UpsertPosition(node string, netMW float64, avgPrice *float64) error {
	_, err := s.db.Exec(`
		INSERT INTO positions (node, net_mw, avg_price)
		VALUES ($1,$2,$3)
		ON CONFLICT (node) DO UPDATE SET net_mw=$2, avg_price=$3, updated_at=now()`,
		node, netMW, avgPrice)
	return err
}

func (s *Store) GetPosition(node string) (*Position, error) {
	var p Position
	err := s.db.QueryRow(`SELECT node,net_mw,avg_price FROM positions WHERE node=$1`, node).
		Scan(&p.Node, &p.NetMW, &p.AvgPrice)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}
