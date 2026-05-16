import argparse

from strategies.base import emit_signal, listen_ticks, load_config


def compute_ema(lmp: float, prev_ema: float, period: int) -> float:
    """Exponential moving average: α*lmp + (1-α)*prev_ema, α = 2/(period+1)."""
    alpha = 2.0 / (period + 1)
    return alpha * lmp + (1.0 - alpha) * prev_ema


def detect_cross(
    fast_prev: float, fast_curr: float,
    slow_prev: float, slow_curr: float,
) -> str | None:
    """Return 'BUY' on fast-crosses-above-slow, 'SELL' on fast-crosses-below, else None."""
    prev_above = fast_prev > slow_prev
    curr_above = fast_curr > slow_curr
    if not prev_above and curr_above:
        return 'BUY'
    if prev_above and not curr_above:
        return 'SELL'
    return None


def run(db_url: str, node: str) -> None:
    cfg = load_config(db_url, 'ma_crossover', node)
    fast_period = int(cfg['fast_period'])
    slow_period = int(cfg['slow_period'])
    quantity_mw = float(cfg['quantity_mw'])

    fast_ema: float | None = None
    slow_ema: float | None = None
    tick_count = 0

    print(f"ma_crossover: node={node} fast={fast_period} slow={slow_period}")

    for tick in listen_ticks(db_url, node):
        lmp = tick['lmp']
        tick_count += 1

        if fast_ema is None:
            fast_ema = slow_ema = lmp
            continue

        new_fast = compute_ema(lmp, fast_ema, fast_period)
        new_slow = compute_ema(lmp, slow_ema, slow_period)

        if tick_count > slow_period:
            side = detect_cross(fast_ema, new_fast, slow_ema, new_slow)
            if side:
                signal_id = emit_signal(db_url, 'ma_crossover', node, side, quantity_mw, lmp)
                print(f"ma_crossover: signal id={signal_id} {side} {quantity_mw}MW @ {lmp:.4f}")

        fast_ema = new_fast
        slow_ema = new_slow


if __name__ == '__main__':
    import os
    p = argparse.ArgumentParser()
    p.add_argument('--node', default='HB_NORTH')
    p.add_argument('--db-url', default=os.getenv('DATABASE_URL',
        'postgresql://postgres:postgres@localhost:5432/trade_infra'))
    args = p.parse_args()
    run(args.db_url, args.node)
