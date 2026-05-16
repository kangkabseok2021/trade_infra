import json
import select
import psycopg2
import psycopg2.extensions


def load_config(db_url: str, strategy: str, node: str) -> dict[str, str]:
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()
    cur.execute(
        "SELECT param_key, param_value FROM strategy_configs WHERE strategy=%s AND node=%s",
        (strategy, node),
    )
    cfg = {row[0]: row[1] for row in cur.fetchall()}
    conn.close()
    return cfg


def listen_ticks(db_url: str, node: str):
    """Yield tick dicts {node, lmp} for the given node via LISTEN price_ticks."""
    conn = psycopg2.connect(db_url)
    conn.set_isolation_level(psycopg2.extensions.ISOLATION_LEVEL_AUTOCOMMIT)
    cur = conn.cursor()
    cur.execute("LISTEN price_ticks")
    while True:
        if select.select([conn], [], [], 5.0) == ([], [], []):
            continue
        conn.poll()
        while conn.notifies:
            notify = conn.notifies.pop(0)
            try:
                payload = json.loads(notify.payload)
            except json.JSONDecodeError:
                continue
            if payload.get("node") == node:
                yield payload


def emit_signal(
    db_url: str,
    strategy: str,
    node: str,
    side: str,
    quantity_mw: float,
    limit_price: float,
) -> int:
    """Insert a signal row and NOTIFY 'signals'. Returns the signal id."""
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(
        """
        INSERT INTO signals (strategy, node, side, quantity_mw, limit_price)
        VALUES (%s, %s, %s, %s, %s)
        RETURNING id
        """,
        (strategy, node, side, round(quantity_mw, 2), round(limit_price, 4)),
    )
    signal_id = cur.fetchone()[0]
    cur.execute("SELECT pg_notify('signals', %s)", (str(signal_id),))
    conn.close()
    return signal_id
