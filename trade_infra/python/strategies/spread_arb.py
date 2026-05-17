import argparse
import statistics
from collections import deque

from strategies.base import emit_signal, listen_ticks_multi, load_config


def compute_spread_signal(
    spreads: list[float],
    window: int,
    threshold: float,
) -> tuple[str, str] | None:
    """Return (side_a, side_b) or None based on z-score of the rolling spread series.

    side_a/side_b are 'BUY' or 'SELL' for node_a and node_b respectively.
    """
    if len(spreads) < window:
        return None
    window_data = spreads[-window:]
    if len(window_data) < 2:
        return None
    mean = statistics.mean(window_data)
    std = statistics.stdev(window_data)
    if std == 0:
        return None
    z = (window_data[-1] - mean) / std
    if z < -threshold:
        return ('BUY', 'SELL')
    if z > threshold:
        return ('SELL', 'BUY')
    return None


def run(db_url: str, node_a: str, node_b: str) -> None:
    cfg_a = load_config(db_url, 'spread_arb', node_a)
    cfg_b = load_config(db_url, 'spread_arb', node_b)
    window = int(cfg_a['window'])
    threshold = float(cfg_a['threshold'])
    qty_a = float(cfg_a['quantity_mw'])
    qty_b = float(cfg_b['quantity_mw'])

    latest_lmp: dict[str, float] = {}
    spreads: deque[float] = deque(maxlen=window)

    print(f"spread_arb: node_a={node_a} node_b={node_b} window={window} threshold={threshold}")

    for tick in listen_ticks_multi(db_url, {node_a, node_b}):
        latest_lmp[tick['node']] = tick['lmp']
        if node_a not in latest_lmp or node_b not in latest_lmp:
            continue
        spread = latest_lmp[node_a] - latest_lmp[node_b]
        spreads.append(spread)
        result = compute_spread_signal(list(spreads), window, threshold)
        if result:
            side_a, side_b = result
            id_a = emit_signal(db_url, 'spread_arb', node_a, side_a, qty_a, latest_lmp[node_a])
            id_b = emit_signal(db_url, 'spread_arb', node_b, side_b, qty_b, latest_lmp[node_b])
            print(
                f"spread_arb: signal ids={id_a},{id_b} "
                f"{side_a} {node_a} / {side_b} {node_b}"
            )


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node-a', default='HB_NORTH')
    p.add_argument('--node-b', default='HB_SOUTH')
    p.add_argument('--db-url', default=os.getenv(
        'DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra',
    ))
    args = p.parse_args()
    run(args.db_url, args.node_a, args.node_b)
