CREATE TABLE IF NOT EXISTS price_ticks (
    id          BIGSERIAL PRIMARY KEY,
    node        TEXT NOT NULL,
    lmp         NUMERIC(10,4) NOT NULL,
    load_mw     NUMERIC(10,2),
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_price_ticks_node_ts ON price_ticks (node, timestamp DESC);

CREATE TABLE IF NOT EXISTS orders (
    id          BIGSERIAL PRIMARY KEY,
    node        TEXT NOT NULL,
    side        TEXT NOT NULL CHECK (side IN ('BUY','SELL')),
    quantity_mw NUMERIC(10,2) NOT NULL CHECK (quantity_mw > 0),
    limit_price NUMERIC(10,4) NOT NULL CHECK (limit_price > 0),
    status      TEXT NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING','SUBMITTED','PARTIALLY_FILLED',
                                      'FILLED','REJECTED','CANCELLED')),
    filled_at   NUMERIC(10,4),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_orders_node_status ON orders (node, status);

CREATE TABLE IF NOT EXISTS positions (
    id          BIGSERIAL PRIMARY KEY,
    node        TEXT UNIQUE NOT NULL,
    net_mw      NUMERIC(10,2) NOT NULL DEFAULT 0,
    avg_price   NUMERIC(10,4),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS risk_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    node            TEXT NOT NULL,
    mtm_pnl         NUMERIC(14,4) NOT NULL,
    net_exposure_mw NUMERIC(10,2) NOT NULL,
    limit_headroom  NUMERIC(10,2) NOT NULL,
    snapshot_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_risk_snapshots_node_ts ON risk_snapshots (node, snapshot_at DESC);
