import argparse
import psycopg2


def format_pnl_report(node: str, mtm_pnl: float, net_exposure_mw: float, limit_headroom: float) -> str:
    status = "BREACH" if limit_headroom < 0 else "WITHIN LIMITS"
    return (f"[{node}] MTM P&L: ${mtm_pnl:.2f} | "
            f"Net Exposure: {net_exposure_mw:.1f} MW | "
            f"Headroom: {limit_headroom:.1f} MW | {status}")


def format_position_summary(node: str, net_mw: float, avg_price: float | None) -> str:
    if net_mw > 0:
        direction = "LONG"
    elif net_mw < 0:
        direction = "SHORT"
    else:
        direction = "FLAT"
    price_str = f"avg=${avg_price:.4f}" if avg_price is not None else "no fills"
    return f"[{node}] {direction} {abs(net_mw):.1f} MW @ {price_str}"


def print_report(db_url: str) -> None:
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    print("\n=== Positions ===")
    cur.execute("SELECT node, net_mw, avg_price FROM positions ORDER BY node")
    for node, net_mw, avg_price in cur.fetchall():
        print(format_position_summary(node, float(net_mw), float(avg_price) if avg_price else None))

    print("\n=== Latest Risk Snapshots ===")
    cur.execute("""
        SELECT DISTINCT ON (node) node, mtm_pnl, net_exposure_mw, limit_headroom
        FROM risk_snapshots ORDER BY node, snapshot_at DESC
    """)
    for node, pnl, exp, headroom in cur.fetchall():
        print(format_pnl_report(node, float(pnl), float(exp), float(headroom)))

    print("\n=== Recent Price Ticks ===")
    cur.execute("""
        SELECT DISTINCT ON (node) node, lmp, timestamp
        FROM price_ticks ORDER BY node, timestamp DESC
    """)
    for node, lmp, ts in cur.fetchall():
        print(f"[{node}] LMP=${float(lmp):.4f}/MWh at {ts}")

    conn.close()


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--db-url", default="postgresql://postgres:postgres@localhost:5432/trade_infra")
    args = p.parse_args()
    print_report(args.db_url)
