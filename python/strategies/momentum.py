import argparse
from collections import deque

from strategies.base import emit_signal, listen_ticks, load_config


def compute_momentum_signal(
    lmps: list[float],
    window: int,
    threshold_pct: float,
) -> str | None:
    """Return 'BUY', 'SELL', or None based on Rate-of-Change over window ticks."""
    if len(lmps) < window + 1:
        return None
    base = lmps[-window - 1]
    if base == 0:
        return None
    roc = (lmps[-1] - base) / base * 100
    if roc > threshold_pct:
        return 'BUY'
    if roc < -threshold_pct:
        return 'SELL'
    return None


def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'momentum', node)
    window = int(cfg['window'])
    threshold_pct = float(cfg['threshold_pct'])
    quantity_mw = float(cfg['quantity_mw'])

    buf: deque[float] = deque(maxlen=window + 1)

    print(f"momentum: node={node} window={window} threshold_pct={threshold_pct}")

    for tick in listen_ticks(db_url, node):
        buf.append(tick['lmp'])
        side = compute_momentum_signal(list(buf), window, threshold_pct)
        if side:
            signal_id = emit_signal(db_url, 'momentum', node, side, quantity_mw, tick['lmp'])
            print(f"momentum: signal id={signal_id} {side} {quantity_mw}MW @ {tick['lmp']:.4f}")


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node', default='HB_NORTH')
    p.add_argument('--db-url', default=os.getenv(
        'DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra',
    ))
    args = p.parse_args()
    run(args.db_url, args.node)
