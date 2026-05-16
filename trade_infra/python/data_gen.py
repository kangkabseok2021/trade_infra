import argparse
import random
import psycopg2
from datetime import datetime, timedelta


def generate_price_ticks(node: str, base_lmp: float, volatility: float,
                         n: int, seed: int = 42) -> list[dict]:
    rng = random.Random(seed)
    lmp = base_lmp
    ticks = []
    for _ in range(n):
        lmp += 0.1 * (base_lmp - lmp) + volatility * rng.gauss(0, 1)
        lmp = max(1.0, min(lmp, 499.0))
        load_mw = 15000.0 + rng.gauss(0, 500.0)
        ticks.append({"node": node, "lmp": round(lmp, 4), "load_mw": round(load_mw, 2)})
    return ticks


NODE_CONFIGS: dict[str, tuple[float, float]] = {
    "HB_NORTH":   (45.0, 8.0),
    "HB_SOUTH":   (42.0, 7.0),
    "HB_WEST":    (38.0, 6.0),
    "HB_HOUSTON": (47.0, 9.0),
}


def seed_database(db_url: str, nodes: list[str], ticks_per_node: int, seed: int = 42) -> int:
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()
    base_time = datetime.utcnow() - timedelta(seconds=ticks_per_node)
    total = 0
    for node in nodes:
        base_lmp, vol = NODE_CONFIGS.get(node, (40.0, 7.0))
        for i, tick in enumerate(generate_price_ticks(node, base_lmp, vol, ticks_per_node, seed)):
            cur.execute(
                "INSERT INTO price_ticks (node, lmp, load_mw, timestamp) VALUES (%s, %s, %s, %s)",
                (tick["node"], tick["lmp"], tick["load_mw"], base_time + timedelta(seconds=i)),
            )
            total += 1
    conn.commit()
    conn.close()
    return total


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--nodes", default="HB_NORTH,HB_SOUTH,HB_WEST,HB_HOUSTON")
    p.add_argument("--ticks", type=int, default=3600)
    p.add_argument("--db-url", default="postgresql://postgres:postgres@localhost:5432/trade_infra")
    p.add_argument("--seed", type=int, default=42)
    args = p.parse_args()
    nodes = [n.strip() for n in args.nodes.split(",")]
    count = seed_database(args.db_url, nodes, args.ticks, args.seed)
    print(f"Seeded {count} ticks across {len(nodes)} nodes")
