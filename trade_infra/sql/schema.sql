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

CREATE TABLE IF NOT EXISTS signals (
    id           BIGSERIAL PRIMARY KEY,
    strategy     TEXT NOT NULL,
    node         TEXT NOT NULL,
    side         TEXT NOT NULL CHECK (side IN ('BUY','SELL')),
    quantity_mw  NUMERIC(10,2) NOT NULL,
    limit_price  NUMERIC(10,4) NOT NULL,
    status       TEXT NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING','SUBMITTED','SKIPPED')),
    reason       TEXT,
    order_id     BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_signals_strategy_node_ts
    ON signals (strategy, node, created_at DESC);

CREATE TABLE IF NOT EXISTS strategy_configs (
    strategy    TEXT NOT NULL,
    node        TEXT NOT NULL,
    param_key   TEXT NOT NULL,
    param_value TEXT NOT NULL,
    PRIMARY KEY (strategy, node, param_key)
);

INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('mean_reversion', 'HB_NORTH', 'window',       '20'),
    ('mean_reversion', 'HB_NORTH', 'threshold',    '1.0'),
    ('mean_reversion', 'HB_NORTH', 'quantity_mw',  '5.0'),
    ('ma_crossover',   'HB_NORTH', 'fast_period',  '5'),
    ('ma_crossover',   'HB_NORTH', 'slow_period',  '20'),
    ('ma_crossover',   'HB_NORTH', 'quantity_mw',  '5.0')
ON CONFLICT DO NOTHING;

-- HB_SOUTH strategy config
INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('mean_reversion', 'HB_SOUTH', 'window',       '20'),
    ('mean_reversion', 'HB_SOUTH', 'threshold',    '1.0'),
    ('mean_reversion', 'HB_SOUTH', 'quantity_mw',  '5.0'),
    ('ma_crossover',   'HB_SOUTH', 'fast_period',  '5'),
    ('ma_crossover',   'HB_SOUTH', 'slow_period',  '20'),
    ('ma_crossover',   'HB_SOUTH', 'quantity_mw',  '5.0'),
-- HB_WEST strategy config (higher threshold + smaller qty — lower volatility node)
    ('mean_reversion', 'HB_WEST',  'window',       '20'),
    ('mean_reversion', 'HB_WEST',  'threshold',    '1.2'),
    ('mean_reversion', 'HB_WEST',  'quantity_mw',  '4.0'),
    ('ma_crossover',   'HB_WEST',  'fast_period',  '5'),
    ('ma_crossover',   'HB_WEST',  'slow_period',  '20'),
    ('ma_crossover',   'HB_WEST',  'quantity_mw',  '4.0')
ON CONFLICT DO NOTHING;

-- Spread-arb strategy config (HB_NORTH / HB_SOUTH pair)
INSERT INTO strategy_configs (strategy, node, param_key, param_value) VALUES
    ('spread_arb', 'HB_NORTH', 'window',       '20'),
    ('spread_arb', 'HB_NORTH', 'threshold',    '1.5'),
    ('spread_arb', 'HB_NORTH', 'quantity_mw',  '5.0'),
    ('spread_arb', 'HB_SOUTH', 'window',       '20'),
    ('spread_arb', 'HB_SOUTH', 'threshold',    '1.5'),
    ('spread_arb', 'HB_SOUTH', 'quantity_mw',  '5.0')
ON CONFLICT DO NOTHING;
