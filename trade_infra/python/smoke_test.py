"""Integration smoke test: runs against a live docker-compose stack."""
import sys
import time
import psycopg2
import requests


ORDER_SVC       = "http://localhost:8080"
RISK_SVC        = "http://localhost:8081"
STRATEGY_ENGINE = "http://localhost:9104"
DB_URL          = "postgresql://postgres:postgres@localhost:5432/trade_infra"


def wait_for(url: str, timeout: int = 60) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            if requests.get(url, timeout=2).status_code == 200:
                return
        except Exception:
            pass
        time.sleep(2)
    raise TimeoutError(f"{url} not healthy after {timeout}s")


def main() -> int:
    print("Waiting for services to be healthy...")
    wait_for(f"{ORDER_SVC}/health")
    wait_for(f"{RISK_SVC}/health")
    wait_for(f"{STRATEGY_ENGINE}/health")
    print("strategy-engine healthy.")
    print("All services healthy.")

    # Wait for price ticks to accumulate
    time.sleep(5)
    conn = psycopg2.connect(DB_URL)
    cur = conn.cursor()

    cur.execute("SELECT COUNT(*) FROM price_ticks")
    tick_count = cur.fetchone()[0]
    print(f"Price ticks in DB: {tick_count}")
    assert tick_count > 0, "No price ticks — market-data-svc not running"

    cur.execute("SELECT lmp FROM price_ticks WHERE node='HB_NORTH' ORDER BY timestamp DESC LIMIT 1")
    row = cur.fetchone()
    assert row is not None, "No HB_NORTH ticks found"
    current_lmp = float(row[0])
    print(f"Current HB_NORTH LMP: ${current_lmp:.4f}/MWh")

    # Submit a BUY order with limit above current LMP — should fill on next tick
    limit_price = current_lmp + 10.0
    resp = requests.post(f"{ORDER_SVC}/orders", json={
        "node": "HB_NORTH",
        "side": "BUY",
        "quantity_mw": 5.0,
        "limit_price": limit_price,
    })
    assert resp.status_code == 201, f"Order creation failed: {resp.status_code} {resp.text}"
    order_id = resp.json()["id"]
    print(f"Created order {order_id} with limit=${limit_price:.4f}")

    # Wait up to 30s for the order to fill
    deadline = time.time() + 30
    filled = False
    while time.time() < deadline:
        r = requests.get(f"{ORDER_SVC}/orders/{order_id}")
        if r.json().get("status") == "FILLED":
            filled = True
            break
        time.sleep(2)

    assert filled, f"Order {order_id} not filled within 30s"
    print(f"Order {order_id} filled successfully.")

    # Verify risk snapshot was created
    time.sleep(3)
    cur.execute("SELECT COUNT(*) FROM risk_snapshots WHERE node='HB_NORTH'")
    snap_count = cur.fetchone()[0]
    assert snap_count > 0, "No risk snapshot created after fill"
    print(f"Risk snapshots for HB_NORTH: {snap_count}")

    print("\n=== Waiting for a strategy signal ===")
    deadline = time.time() + 120  # strategies need warm-up time (20-tick window)
    signal_submitted = False
    while time.time() < deadline:
        cur.execute(
            "SELECT id, strategy, side, status, order_id FROM signals "
            "WHERE status='SUBMITTED' AND order_id IS NOT NULL LIMIT 1"
        )
        row = cur.fetchone()
        if row:
            sig_id, strategy, side, status, order_id = row
            print(f"Strategy signal: id={sig_id} strategy={strategy} side={side} order_id={order_id}")
            signal_submitted = True
            break
        time.sleep(3)

    assert signal_submitted, "No submitted signal with order_id within 120s — check mean_reversion and strategy-engine logs"
    print("✓ Strategy signal submitted and order placed")

    conn.close()
    print("\n✓ Smoke test PASSED")
    return 0


if __name__ == "__main__":
    sys.exit(main())
