import argparse
import statistics
from collections import deque

from strategies.base import emit_signal, listen_ticks, load_config


def compute_signal(lmps: list[float], window: int, threshold: float) -> str | None:
    """Return 'BUY', 'SELL', or None based on mean-reversion logic."""
    if len(lmps) < window:
        return None
    window_data = lmps[-window:]
    if len(window_data) < 2:
        return None
    mean = statistics.mean(window_data)
    std = statistics.stdev(window_data)
    if std == 0:
        return None
    current = window_data[-1]
    if current < mean - threshold * std:
        return 'BUY'
    if current > mean + threshold * std:
        return 'SELL'
    return None


def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'mean_reversion', node)
    window = int(cfg['window'])
    threshold = float(cfg['threshold'])
    quantity_mw = float(cfg['quantity_mw'])

    buf: deque[float] = deque(maxlen=window)
    print(f"mean_reversion: node={node} window={window} threshold={threshold}")

    for tick in listen_ticks(db_url, node):
        buf.append(tick['lmp'])
        side = compute_signal(list(buf), window, threshold)
        if side:
            limit_price = tick['lmp']
            signal_id = emit_signal(db_url, 'mean_reversion', node, side, quantity_mw, limit_price)
            print(f"mean_reversion: signal id={signal_id} {side} {quantity_mw}MW @ {limit_price:.4f}")


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node', default='HB_NORTH')
    p.add_argument('--db-url', default=os.getenv('DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra'))
    args = p.parse_args()
    run(args.db_url, args.node)
