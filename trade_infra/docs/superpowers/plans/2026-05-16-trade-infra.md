# trade_infra Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a four-service energy trading infrastructure (market data, orders, risk/P&L, monitoring) that mirrors real-world trading system developer daily work.

**Architecture:** C++ shared libraries (`libmarketdata.so`, `libriskcalc.so`) with clean `extern "C"` ABI for all computation; Go microservices for HTTP APIs and PostgreSQL LISTEN/NOTIFY orchestration; Python (uv) for synthetic data generation and analytics; Prometheus + Grafana for observability. All services communicate through PostgreSQL.

**Tech Stack:** C++17 + CMake 3.20 + GoogleTest 1.14 + cpp-httplib + libpq, Go 1.22 + lib/pq + prometheus/client_golang, Python 3.12 + uv + psycopg2 + pytest, PostgreSQL 16, Prometheus 2.x, Grafana 10.x, Docker Compose, GitHub Actions.

---

## File Map

```
trade_infra/
├── .gitignore
├── sql/schema.sql
├── python/
│   ├── pyproject.toml
│   ├── data_gen.py
│   ├── analytics.py
│   └── tests/
│       ├── test_data_gen.py
│       └── test_analytics.py
├── market-data-svc/
│   ├── CMakeLists.txt
│   ├── include/marketdata.h          ← extern "C" ABI (Rust-swappable)
│   ├── src/
│   │   ├── tick_generator.h
│   │   ├── tick_generator.cpp
│   │   ├── db_writer.h
│   │   ├── db_writer.cpp
│   │   ├── metrics_server.h
│   │   ├── metrics_server.cpp
│   │   └── main.cpp
│   └── tests/test_tick_generator.cpp
├── order-svc/
│   ├── go.mod
│   ├── internal/
│   │   ├── order/
│   │   │   ├── model.go
│   │   │   ├── store.go
│   │   │   └── store_test.go
│   │   ├── evaluator/
│   │   │   ├── evaluator.go
│   │   │   └── evaluator_test.go
│   │   ├── handler/
│   │   │   ├── handler.go
│   │   │   └── handler_test.go
│   │   └── listener/
│   │       └── listener.go
│   ├── metrics/metrics.go
│   └── cmd/server/main.go
├── risk-svc/
│   ├── CMakeLists.txt
│   ├── include/riskcalc.h            ← extern "C" ABI (Rust-swappable)
│   ├── src/
│   │   ├── risk_calc.h
│   │   └── risk_calc.cpp
│   ├── tests/test_risk_calc.cpp
│   ├── internal/
│   │   ├── riskcalc/riskcalc.go     ← CGo wrapper
│   │   ├── risk/
│   │   │   ├── model.go
│   │   │   ├── store.go
│   │   │   └── store_test.go
│   │   ├── handler/handler.go
│   │   └── listener/listener.go
│   ├── metrics/metrics.go
│   └── cmd/server/main.go
├── infra/
│   ├── docker-compose.yml
│   ├── prometheus.yml
│   ├── slo_rules.yml
│   └── grafana/dashboards/trade_infra.json
└── .github/workflows/
    ├── market-data-svc.yml
    ├── order-svc.yml
    ├── risk-svc.yml
    └── integration.yml
```

---

## Phase 1: Foundation — Schema, Python Tools, market-data-svc

### Task 1: Git remote + scaffold + .gitignore

**Files:** `.gitignore`

- [ ] **Set git remote**

```bash
git remote add origin git@github.com:kangkabseok2021/trade_infra.git
git remote -v
```

Expected:
```
origin  git@github.com:kangkabseok2021/trade_infra.git (fetch)
origin  git@github.com:kangkabseok2021/trade_infra.git (push)
```

- [ ] **Create directory structure**

```bash
mkdir -p market-data-svc/{src,include,tests}
mkdir -p order-svc/{cmd/server,internal/{order,evaluator,handler,listener},metrics}
mkdir -p risk-svc/{src,include,tests,cmd/server,internal/{riskcalc,risk,handler,listener},metrics}
mkdir -p python/tests
mkdir -p infra/grafana/dashboards
mkdir -p sql
mkdir -p .github/workflows
```

- [ ] **Create `.gitignore`**

```
build/
*.o
*.so
*.dylib
*.a
__pycache__/
*.pyc
.venv/
CMakeFiles/
CMakeCache.txt
cmake_install.cmake
.DS_Store
```

- [ ] **Commit**

```bash
git add .gitignore
git commit -m "chore: project scaffold and git remote"
```

---

### Task 2: PostgreSQL schema

**Files:** `sql/schema.sql`

- [ ] **Write schema**

Create `sql/schema.sql`:

```sql
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
```

- [ ] **Apply to local PostgreSQL**

```bash
createdb trade_infra 2>/dev/null || true
createdb trade_infra_test 2>/dev/null || true
psql trade_infra -f sql/schema.sql
psql trade_infra_test -f sql/schema.sql
psql trade_infra -c "\dt"
```

Expected: 4 tables listed (orders, positions, price_ticks, risk_snapshots).

- [ ] **Commit**

```bash
git add sql/schema.sql
git commit -m "feat(sql): schema — price_ticks, orders, positions, risk_snapshots"
```

---

### Task 3: Python uv project + data_gen.py (TDD)

**Files:** `python/pyproject.toml`, `python/tests/test_data_gen.py`, `python/data_gen.py`

- [ ] **Write pyproject.toml**

Create `python/pyproject.toml`:

```toml
[project]
name = "trade-infra-tools"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "psycopg2-binary>=2.9",
    "pandas>=2.2",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "ruff>=0.4",
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
```

- [ ] **Sync uv environment**

```bash
cd python && uv sync --extra dev && uv run pytest --version
```

Expected: `pytest 8.x.x`

- [ ] **Write failing tests**

Create `python/tests/test_data_gen.py`:

```python
import sys, os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
from data_gen import generate_price_ticks

def test_returns_correct_count():
    ticks = generate_price_ticks("HB_NORTH", base_lmp=45.0, volatility=5.0, n=10, seed=42)
    assert len(ticks) == 10

def test_tick_has_required_fields():
    ticks = generate_price_ticks("HB_NORTH", base_lmp=45.0, volatility=5.0, n=1, seed=42)
    assert {"node", "lmp", "load_mw"}.issubset(ticks[0].keys())
    assert ticks[0]["node"] == "HB_NORTH"

def test_lmp_stays_in_range():
    ticks = generate_price_ticks("HB_NORTH", base_lmp=45.0, volatility=5.0, n=200, seed=42)
    assert all(0 < t["lmp"] < 500 for t in ticks)

def test_deterministic_with_same_seed():
    t1 = generate_price_ticks("HB_NORTH", 45.0, 5.0, 5, seed=42)
    t2 = generate_price_ticks("HB_NORTH", 45.0, 5.0, 5, seed=42)
    assert [t["lmp"] for t in t1] == [t["lmp"] for t in t2]
```

- [ ] **Run to verify failure**

```bash
cd python && uv run pytest tests/test_data_gen.py -v 2>&1 | head -8
```

Expected: `ModuleNotFoundError: No module named 'data_gen'`

- [ ] **Implement data_gen.py**

Create `python/data_gen.py`:

```python
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
```

- [ ] **Run tests — expect all pass**

```bash
cd python && uv run pytest tests/test_data_gen.py -v
```

Expected:
```
PASSED tests/test_data_gen.py::test_returns_correct_count
PASSED tests/test_data_gen.py::test_tick_has_required_fields
PASSED tests/test_data_gen.py::test_lmp_stays_in_range
PASSED tests/test_data_gen.py::test_deterministic_with_same_seed
4 passed
```

- [ ] **Commit**

```bash
git add python/
git commit -m "feat(python): uv setup + data_gen Ornstein-Uhlenbeck LMP generator (TDD)"
```

---

### Task 4: market-data-svc CMake + extern "C" header

**Files:** `market-data-svc/CMakeLists.txt`, `market-data-svc/include/marketdata.h`

- [ ] **Write extern "C" header**

Create `market-data-svc/include/marketdata.h`:

```cpp
#pragma once
#ifdef __cplusplus
extern "C" {
#endif

typedef struct TickGenerator TickGenerator;

/** Create a tick generator (Ornstein-Uhlenbeck process).
 *  Rust replacement: expose same symbols as #[no_mangle] extern "C" in a cdylib. */
TickGenerator* tick_generator_create(double base_lmp, double volatility, unsigned int seed);
void           tick_generator_destroy(TickGenerator* gen);
void           tick_generator_next(TickGenerator* gen, double* out_lmp, double* out_load_mw);

#ifdef __cplusplus
}
#endif
```

- [ ] **Write CMakeLists.txt**

Create `market-data-svc/CMakeLists.txt`:

```cmake
cmake_minimum_required(VERSION 3.20)
project(market_data_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)

FetchContent_Declare(googletest
    GIT_REPOSITORY https://github.com/google/googletest.git
    GIT_TAG        v1.14.0)
set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

FetchContent_Declare(httplib
    GIT_REPOSITORY https://github.com/yhirose/cpp-httplib.git
    GIT_TAG        v0.15.3)
FetchContent_MakeAvailable(httplib)

find_package(PostgreSQL REQUIRED)

# Shared library — extern "C" ABI, Rust-swappable
add_library(marketdata SHARED src/tick_generator.cpp)
target_include_directories(marketdata PUBLIC include PRIVATE src)
if(APPLE)
    target_link_options(marketdata PRIVATE -undefined dynamic_lookup)
endif()

# Main service binary
add_executable(market_data_svc
    src/main.cpp src/db_writer.cpp src/metrics_server.cpp)
target_include_directories(market_data_svc PRIVATE include src)
target_link_libraries(market_data_svc marketdata httplib::httplib PostgreSQL::PostgreSQL)

# Tests
enable_testing()
add_executable(test_marketdata tests/test_tick_generator.cpp)
target_link_libraries(test_marketdata marketdata GTest::gtest_main)
include(GoogleTest)
gtest_discover_tests(test_marketdata)
```

- [ ] **Commit**

```bash
git add market-data-svc/CMakeLists.txt market-data-svc/include/marketdata.h
git commit -m "feat(market-data-svc): CMake structure and extern-C ABI header"
```

---

### Task 5: TickGenerator — failing GoogleTest suite

**Files:** `market-data-svc/tests/test_tick_generator.cpp`

- [ ] **Write failing tests**

Create `market-data-svc/tests/test_tick_generator.cpp`:

```cpp
#include <gtest/gtest.h>
#include "marketdata.h"

TEST(TickGeneratorTest, CreateAndDestroy) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    ASSERT_NE(gen, nullptr);
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, LMPIsPositive) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    double lmp, load;
    tick_generator_next(gen, &lmp, &load);
    EXPECT_GT(lmp, 0.0);
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, LMPStaysInRange) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    double lmp, load;
    for (int i = 0; i < 1000; ++i) {
        tick_generator_next(gen, &lmp, &load);
        EXPECT_GT(lmp, 0.0) << "negative at tick " << i;
        EXPECT_LT(lmp, 500.0) << "too high at tick " << i;
    }
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, LoadMWInRange) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    double lmp, load;
    tick_generator_next(gen, &lmp, &load);
    EXPECT_GT(load, 5000.0);
    EXPECT_LT(load, 30000.0);
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, DeterministicWithSameSeed) {
    TickGenerator* g1 = tick_generator_create(45.0, 5.0, 42);
    TickGenerator* g2 = tick_generator_create(45.0, 5.0, 42);
    double l1, l2, d1, d2;
    for (int i = 0; i < 20; ++i) {
        tick_generator_next(g1, &l1, &d1);
        tick_generator_next(g2, &l2, &d2);
        EXPECT_DOUBLE_EQ(l1, l2) << "diverged at tick " << i;
    }
    tick_generator_destroy(g1);
    tick_generator_destroy(g2);
}
```

- [ ] **Verify build fails (no implementation yet)**

```bash
cd market-data-svc
cmake -B build -S . && cmake --build build --target test_marketdata 2>&1 | tail -5
```

Expected: linker error — `undefined symbol tick_generator_create`.

- [ ] **Commit failing test**

```bash
git add market-data-svc/tests/test_tick_generator.cpp
git commit -m "test(market-data-svc): GoogleTest suite for TickGenerator (failing)"
```

---

### Task 6: TickGenerator — implement to pass tests

**Files:** `market-data-svc/src/tick_generator.h`, `market-data-svc/src/tick_generator.cpp`

- [ ] **Write internal C++ header**

Create `market-data-svc/src/tick_generator.h`:

```cpp
#pragma once
#include <random>

class TickGenerator {
public:
    TickGenerator(double base_lmp, double volatility, unsigned int seed);
    void next(double& out_lmp, double& out_load_mw);
private:
    double base_lmp_, volatility_, lmp_;
    std::mt19937 rng_;
    std::normal_distribution<double> dist_{0.0, 1.0};
    static constexpr double kTheta = 0.1;
};
```

- [ ] **Write implementation**

Create `market-data-svc/src/tick_generator.cpp`:

```cpp
#include "tick_generator.h"
#include "marketdata.h"
#include <algorithm>

TickGenerator::TickGenerator(double base_lmp, double volatility, unsigned int seed)
    : base_lmp_(base_lmp), volatility_(volatility), lmp_(base_lmp), rng_(seed) {}

void TickGenerator::next(double& out_lmp, double& out_load_mw) {
    lmp_ += kTheta * (base_lmp_ - lmp_) + volatility_ * dist_(rng_);
    lmp_ = std::clamp(lmp_, 1.0, 499.0);
    out_lmp = lmp_;
    out_load_mw = std::max(5001.0, 15000.0 + 500.0 * dist_(rng_));
}

extern "C" {
TickGenerator* tick_generator_create(double base_lmp, double volatility, unsigned int seed) {
    return new TickGenerator(base_lmp, volatility, seed);
}
void tick_generator_destroy(TickGenerator* gen) { delete gen; }
void tick_generator_next(TickGenerator* gen, double* out_lmp, double* out_load_mw) {
    gen->next(*out_lmp, *out_load_mw);
}
}
```

- [ ] **Build and run tests**

```bash
cd market-data-svc
cmake -B build -S . && cmake --build build --target test_marketdata
cd build && ctest --output-on-failure
```

Expected: `5/5 tests passed`.

- [ ] **Commit**

```bash
git add market-data-svc/src/tick_generator.h market-data-svc/src/tick_generator.cpp
git commit -m "feat(market-data-svc): TickGenerator Ornstein-Uhlenbeck — all tests pass"
```

---

### Task 7: market-data-svc — DB writer + metrics server + main

**Files:** `market-data-svc/src/db_writer.h`, `db_writer.cpp`, `metrics_server.h`, `metrics_server.cpp`, `main.cpp`

- [ ] **Write db_writer.h**

Create `market-data-svc/src/db_writer.h`:

```cpp
#pragma once
#include <string>
#include <libpq-fe.h>

class DbWriter {
public:
    explicit DbWriter(const std::string& connstr);
    ~DbWriter();
    void write_tick(const std::string& node, double lmp, double load_mw);
private:
    PGconn* conn_;
};
```

- [ ] **Write db_writer.cpp**

Create `market-data-svc/src/db_writer.cpp`:

```cpp
#include "db_writer.h"
#include <stdexcept>

DbWriter::DbWriter(const std::string& connstr) {
    conn_ = PQconnectdb(connstr.c_str());
    if (PQstatus(conn_) != CONNECTION_OK) {
        std::string err = PQerrorMessage(conn_);
        PQfinish(conn_);
        throw std::runtime_error("DB connect failed: " + err);
    }
}
DbWriter::~DbWriter() { if (conn_) PQfinish(conn_); }

void DbWriter::write_tick(const std::string& node, double lmp, double load_mw) {
    std::string lmp_s  = std::to_string(lmp);
    std::string load_s = std::to_string(load_mw);
    const char* params[] = { node.c_str(), lmp_s.c_str(), load_s.c_str() };
    PGresult* r = PQexecParams(conn_,
        "INSERT INTO price_ticks (node, lmp, load_mw) VALUES ($1,$2::numeric,$3::numeric)",
        3, nullptr, params, nullptr, nullptr, 0);
    if (PQresultStatus(r) != PGRES_COMMAND_OK) {
        std::string err = PQerrorMessage(conn_);
        PQclear(r);
        throw std::runtime_error("INSERT failed: " + err);
    }
    PQclear(r);
    // NOTIFY downstream listeners
    std::string payload = "{\"node\":\"" + node + "\",\"lmp\":" + lmp_s + "}";
    PGresult* nr = PQexec(conn_, ("NOTIFY price_ticks, '" + payload + "'").c_str());
    PQclear(nr);
}
```

- [ ] **Write metrics_server.h**

Create `market-data-svc/src/metrics_server.h`:

```cpp
#pragma once
#include <atomic>
#include <thread>

class MetricsServer {
public:
    explicit MetricsServer(int port);
    ~MetricsServer();
    void increment_tick_count();
private:
    int port_;
    std::atomic<long long> tick_count_{0};
    std::atomic<bool> running_{true};
    std::thread thread_;
    void run();
};
```

- [ ] **Write metrics_server.cpp**

Create `market-data-svc/src/metrics_server.cpp`:

```cpp
#include "metrics_server.h"
#include <httplib.h>

MetricsServer::MetricsServer(int port) : port_(port) {
    thread_ = std::thread([this]{ run(); });
}
MetricsServer::~MetricsServer() {
    running_ = false;
    if (thread_.joinable()) thread_.join();
}
void MetricsServer::increment_tick_count() { ++tick_count_; }

void MetricsServer::run() {
    httplib::Server svr;
    svr.Get("/metrics", [this](const httplib::Request&, httplib::Response& res){
        std::string body =
            "# HELP market_data_tick_total Total price ticks written\n"
            "# TYPE market_data_tick_total counter\n"
            "market_data_tick_total " + std::to_string(tick_count_.load()) + "\n";
        res.set_content(body, "text/plain; version=0.0.4");
    });
    svr.Get("/health", [](const httplib::Request&, httplib::Response& res){
        res.set_content("ok", "text/plain");
    });
    svr.listen("0.0.0.0", port_);
}
```

- [ ] **Write main.cpp**

Create `market-data-svc/src/main.cpp`:

```cpp
#include <iostream>
#include <string>
#include <chrono>
#include <thread>
#include <cstdlib>
#include "marketdata.h"
#include "db_writer.h"
#include "metrics_server.h"

static std::string env(const char* k, const char* d) {
    const char* v = std::getenv(k); return v ? v : d;
}

int main() {
    std::string db_url     = env("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/trade_infra");
    std::string node       = env("NODE_NAME",    "HB_NORTH");
    double base_lmp        = std::stod(env("BASE_LMP",     "45.0"));
    double volatility      = std::stod(env("VOLATILITY",   "5.0"));
    int interval_ms        = std::stoi(env("INTERVAL_MS",  "1000"));
    int metrics_port       = std::stoi(env("METRICS_PORT", "9101"));

    std::cout << "market-data-svc node=" << node
              << " base_lmp=" << base_lmp << " interval=" << interval_ms << "ms\n";

    DbWriter      db(db_url);
    MetricsServer metrics(metrics_port);
    TickGenerator* gen = tick_generator_create(base_lmp, volatility, 42);

    while (true) {
        double lmp, load_mw;
        tick_generator_next(gen, &lmp, &load_mw);
        try {
            db.write_tick(node, lmp, load_mw);
            metrics.increment_tick_count();
        } catch (const std::exception& e) {
            std::cerr << "write error: " << e.what() << "\n";
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(interval_ms));
    }
    tick_generator_destroy(gen);
}
```

- [ ] **Build full binary**

```bash
cd market-data-svc
cmake -B build -S . && cmake --build build --target market_data_svc
```

Expected: `[100%] Linking CXX executable market_data_svc`

- [ ] **Smoke test (requires running PostgreSQL)**

```bash
DATABASE_URL="postgresql://postgres:postgres@localhost:5432/trade_infra" \
NODE_NAME=HB_NORTH INTERVAL_MS=100 \
./market-data-svc/build/market_data_svc &
sleep 2 && psql trade_infra -c "SELECT COUNT(*) FROM price_ticks" && kill %1
```

Expected: count >= 15.

- [ ] **Commit**

```bash
git add market-data-svc/src/
git commit -m "feat(market-data-svc): DB writer, Prometheus metrics, main binary"
```

---

## Phase 2: order-svc

### Task 8: order-svc Go module + order model

**Files:** `order-svc/go.mod`, `order-svc/internal/order/model.go`

- [ ] **Initialize module**

```bash
cd order-svc
go mod init github.com/kangkabseok2021/trade_infra/order-svc
go get github.com/lib/pq@v1.10.9
go get github.com/prometheus/client_golang@v1.19.0
```

- [ ] **Write model.go**

Create `order-svc/internal/order/model.go`:

```go
package order

import "time"

type Side   string
type Status string

const (
    SideBuy  Side = "BUY"
    SideSell Side = "SELL"
)

const (
    StatusPending         Status = "PENDING"
    StatusFilled          Status = "FILLED"
    StatusRejected        Status = "REJECTED"
    StatusCancelled       Status = "CANCELLED"
)

type Order struct {
    ID         int64     `json:"id"`
    Node       string    `json:"node"`
    Side       Side      `json:"side"`
    QuantityMW float64   `json:"quantity_mw"`
    LimitPrice float64   `json:"limit_price"`
    Status     Status    `json:"status"`
    FilledAt   *float64  `json:"filled_at,omitempty"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type CreateOrderRequest struct {
    Node       string  `json:"node"`
    Side       Side    `json:"side"`
    QuantityMW float64 `json:"quantity_mw"`
    LimitPrice float64 `json:"limit_price"`
}
```

- [ ] **Commit**

```bash
git add order-svc/go.mod order-svc/go.sum order-svc/internal/order/model.go
git commit -m "feat(order-svc): Go module init and order model"
```

---

### Task 9: order-svc PostgreSQL store (TDD)

**Files:** `order-svc/internal/order/store_test.go`, `order-svc/internal/order/store.go`

- [ ] **Write failing store tests**

Create `order-svc/internal/order/store_test.go`:

```go
package order_test

import (
    "database/sql"
    "os"
    "testing"

    _ "github.com/lib/pq"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func testDB(t *testing.T) *sql.DB {
    t.Helper()
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" {
        url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test"
    }
    db, err := sql.Open("postgres", url)
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    t.Cleanup(func() { db.Exec("DELETE FROM orders"); db.Close() })
    return db
}

func TestStore_Create(t *testing.T) {
    s := order.NewStore(testDB(t))
    o, err := s.Create(order.CreateOrderRequest{
        Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 10, LimitPrice: 50,
    })
    if err != nil { t.Fatalf("create: %v", err) }
    if o.ID == 0 { t.Error("expected non-zero ID") }
    if o.Status != order.StatusPending { t.Errorf("want PENDING got %s", o.Status) }
}

func TestStore_GetByID(t *testing.T) {
    s := order.NewStore(testDB(t))
    created, _ := s.Create(order.CreateOrderRequest{
        Node: "HB_SOUTH", Side: order.SideSell, QuantityMW: 5, LimitPrice: 40,
    })
    got, err := s.GetByID(created.ID)
    if err != nil { t.Fatalf("get: %v", err) }
    if got.Node != "HB_SOUTH" { t.Errorf("want HB_SOUTH got %s", got.Node) }
}

func TestStore_ListPendingByNode(t *testing.T) {
    s := order.NewStore(testDB(t))
    s.Create(order.CreateOrderRequest{Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 10, LimitPrice: 50})
    s.Create(order.CreateOrderRequest{Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 5,  LimitPrice: 45})
    s.Create(order.CreateOrderRequest{Node: "HB_SOUTH", Side: order.SideBuy, QuantityMW: 8,  LimitPrice: 42})
    orders, err := s.ListPendingByNode("HB_NORTH")
    if err != nil { t.Fatalf("list: %v", err) }
    if len(orders) != 2 { t.Errorf("want 2 got %d", len(orders)) }
}

func TestStore_UpdateStatus(t *testing.T) {
    s := order.NewStore(testDB(t))
    o, _ := s.Create(order.CreateOrderRequest{
        Node: "HB_NORTH", Side: order.SideBuy, QuantityMW: 10, LimitPrice: 50,
    })
    fa := 48.5
    if err := s.UpdateStatus(o.ID, order.StatusFilled, &fa); err != nil {
        t.Fatalf("update: %v", err)
    }
    got, _ := s.GetByID(o.ID)
    if got.Status != order.StatusFilled { t.Errorf("want FILLED got %s", got.Status) }
    if got.FilledAt == nil || *got.FilledAt != 48.5 { t.Errorf("bad filled_at: %v", got.FilledAt) }
}
```

- [ ] **Run to verify failure**

```bash
cd order-svc
go test ./internal/order/... 2>&1 | head -5
```

Expected: compile error — `store.go` not found.

- [ ] **Implement store.go**

Create `order-svc/internal/order/store.go`:

```go
package order

import (
    "database/sql"
    "time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Create(req CreateOrderRequest) (*Order, error) {
    var o Order
    err := s.db.QueryRow(`
        INSERT INTO orders (node, side, quantity_mw, limit_price)
        VALUES ($1,$2,$3,$4)
        RETURNING id,node,side,quantity_mw,limit_price,status,filled_at,created_at,updated_at`,
        req.Node, string(req.Side), req.QuantityMW, req.LimitPrice,
    ).Scan(&o.ID, &o.Node, &o.Side, &o.QuantityMW, &o.LimitPrice,
        &o.Status, &o.FilledAt, &o.CreatedAt, &o.UpdatedAt)
    return &o, err
}

func (s *Store) GetByID(id int64) (*Order, error) {
    var o Order
    err := s.db.QueryRow(`
        SELECT id,node,side,quantity_mw,limit_price,status,filled_at,created_at,updated_at
        FROM orders WHERE id=$1`, id,
    ).Scan(&o.ID, &o.Node, &o.Side, &o.QuantityMW, &o.LimitPrice,
        &o.Status, &o.FilledAt, &o.CreatedAt, &o.UpdatedAt)
    if err == sql.ErrNoRows { return nil, nil }
    return &o, err
}

func (s *Store) ListPendingByNode(node string) ([]*Order, error) {
    rows, err := s.db.Query(`
        SELECT id,node,side,quantity_mw,limit_price,status,filled_at,created_at,updated_at
        FROM orders WHERE node=$1 AND status='PENDING' ORDER BY created_at`, node)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []*Order
    for rows.Next() {
        var o Order
        if err := rows.Scan(&o.ID, &o.Node, &o.Side, &o.QuantityMW, &o.LimitPrice,
            &o.Status, &o.FilledAt, &o.CreatedAt, &o.UpdatedAt); err != nil {
            return nil, err
        }
        out = append(out, &o)
    }
    return out, rows.Err()
}

func (s *Store) UpdateStatus(id int64, status Status, filledAt *float64) error {
    _, err := s.db.Exec(`
        UPDATE orders SET status=$1,filled_at=$2,updated_at=$3 WHERE id=$4`,
        string(status), filledAt, time.Now().UTC(), id)
    return err
}
```

- [ ] **Run tests**

```bash
cd order-svc
TEST_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/trade_infra_test" \
go test ./internal/order/... -v
```

Expected: 4 tests PASS.

- [ ] **Commit**

```bash
git add order-svc/internal/order/store.go order-svc/internal/order/store_test.go
git commit -m "feat(order-svc): PostgreSQL store with CRUD (TDD)"
```

---

### Task 10: order-svc evaluator — fill logic (TDD)

**Files:** `order-svc/internal/evaluator/evaluator_test.go`, `order-svc/internal/evaluator/evaluator.go`

- [ ] **Write failing tests**

Create `order-svc/internal/evaluator/evaluator_test.go`:

```go
package evaluator_test

import (
    "testing"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/evaluator"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func makeOrder(side order.Side, limitPrice float64) *order.Order {
    return &order.Order{ID: 1, Side: side, QuantityMW: 10, LimitPrice: limitPrice, Status: order.StatusPending}
}

func TestShouldFill_BuyBelowLimit(t *testing.T) {
    o := makeOrder(order.SideBuy, 50.0)
    if !evaluator.ShouldFill(o, 48.0) {
        t.Error("BUY at 48 should fill with limit 50")
    }
}

func TestShouldFill_BuyAboveLimit(t *testing.T) {
    o := makeOrder(order.SideBuy, 50.0)
    if evaluator.ShouldFill(o, 52.0) {
        t.Error("BUY at 52 should NOT fill with limit 50")
    }
}

func TestShouldFill_SellAboveLimit(t *testing.T) {
    o := makeOrder(order.SideSell, 40.0)
    if !evaluator.ShouldFill(o, 42.0) {
        t.Error("SELL at 42 should fill with limit 40")
    }
}

func TestShouldFill_SellBelowLimit(t *testing.T) {
    o := makeOrder(order.SideSell, 40.0)
    if evaluator.ShouldFill(o, 38.0) {
        t.Error("SELL at 38 should NOT fill with limit 40")
    }
}

func TestShouldFill_AtExactLimit(t *testing.T) {
    if !evaluator.ShouldFill(makeOrder(order.SideBuy, 50.0), 50.0) {
        t.Error("BUY at exactly limit should fill")
    }
    if !evaluator.ShouldFill(makeOrder(order.SideSell, 40.0), 40.0) {
        t.Error("SELL at exactly limit should fill")
    }
}
```

- [ ] **Run to verify failure**

```bash
cd order-svc && go test ./internal/evaluator/... 2>&1 | head -5
```

Expected: compile error — `evaluator.go` missing.

- [ ] **Implement evaluator.go**

Create `order-svc/internal/evaluator/evaluator.go`:

```go
package evaluator

import "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"

// ShouldFill returns true when the current market LMP meets the order's limit price.
// BUY fills when lmp <= limit (we can buy cheaper or at limit).
// SELL fills when lmp >= limit (we can sell at or above our floor).
func ShouldFill(o *order.Order, currentLMP float64) bool {
    switch o.Side {
    case order.SideBuy:
        return currentLMP <= o.LimitPrice
    case order.SideSell:
        return currentLMP >= o.LimitPrice
    default:
        return false
    }
}
```

- [ ] **Run tests**

```bash
cd order-svc && go test ./internal/evaluator/... -v
```

Expected: 5 tests PASS.

- [ ] **Commit**

```bash
git add order-svc/internal/evaluator/
git commit -m "feat(order-svc): order fill evaluator — BUY/SELL limit logic (TDD)"
```

---

### Task 11: order-svc REST handler (TDD)

**Files:** `order-svc/internal/handler/handler_test.go`, `order-svc/internal/handler/handler.go`

- [ ] **Write failing handler tests**

Create `order-svc/internal/handler/handler_test.go`:

```go
package handler_test

import (
    "bytes"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    _ "github.com/lib/pq"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/handler"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func testHandler(t *testing.T) *handler.Handler {
    t.Helper()
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" { url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test" }
    db, err := sql.Open("postgres", url)
    if err != nil { t.Fatalf("db: %v", err) }
    t.Cleanup(func() { db.Exec("DELETE FROM orders"); db.Close() })
    return handler.New(order.NewStore(db))
}

func TestCreateOrder_Returns201(t *testing.T) {
    h := testHandler(t)
    body, _ := json.Marshal(map[string]any{
        "node": "HB_NORTH", "side": "BUY", "quantity_mw": 10.0, "limit_price": 50.0,
    })
    req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    h.ServeHTTP(w, req)
    if w.Code != http.StatusCreated {
        t.Errorf("want 201 got %d: %s", w.Code, w.Body)
    }
    var o order.Order
    json.NewDecoder(w.Body).Decode(&o)
    if o.ID == 0 { t.Error("expected ID in response") }
}

func TestGetOrder_Returns200(t *testing.T) {
    h := testHandler(t)
    body, _ := json.Marshal(map[string]any{
        "node": "HB_SOUTH", "side": "SELL", "quantity_mw": 5.0, "limit_price": 40.0,
    })
    req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    h.ServeHTTP(w, req)
    var created order.Order
    json.NewDecoder(w.Body).Decode(&created)

    req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%d", created.ID), nil)
    w2 := httptest.NewRecorder()
    h.ServeHTTP(w2, req2)
    if w2.Code != http.StatusOK { t.Errorf("want 200 got %d", w2.Code) }
}

func TestCreateOrder_InvalidSide_Returns400(t *testing.T) {
    h := testHandler(t)
    body, _ := json.Marshal(map[string]any{
        "node": "HB_NORTH", "side": "HOLD", "quantity_mw": 10.0, "limit_price": 50.0,
    })
    req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    h.ServeHTTP(w, req)
    if w.Code != http.StatusBadRequest { t.Errorf("want 400 got %d", w.Code) }
}
```

- [ ] **Run to verify failure**

```bash
cd order-svc && go test ./internal/handler/... 2>&1 | head -5
```

Expected: compile error.

- [ ] **Implement handler.go**

Create `order-svc/internal/handler/handler.go`:

```go
package handler

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

type Handler struct {
    store *order.Store
    mux   *http.ServeMux
}

func New(store *order.Store) *Handler {
    h := &Handler{store: store, mux: http.NewServeMux()}
    h.mux.HandleFunc("POST /orders", h.createOrder)
    h.mux.HandleFunc("GET /orders/{id}", h.getOrder)
    h.mux.HandleFunc("GET /orders", h.listOrders)
    return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
    var req order.CreateOrderRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid JSON", http.StatusBadRequest)
        return
    }
    if req.Side != order.SideBuy && req.Side != order.SideSell {
        http.Error(w, "side must be BUY or SELL", http.StatusBadRequest)
        return
    }
    if req.QuantityMW <= 0 || req.LimitPrice <= 0 {
        http.Error(w, "quantity_mw and limit_price must be positive", http.StatusBadRequest)
        return
    }
    o, err := h.store.Create(req)
    if err != nil {
        http.Error(w, "db error", http.StatusInternalServerError)
        return
    }
    writeJSON(w, http.StatusCreated, o)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid id", http.StatusBadRequest)
        return
    }
    o, err := h.store.GetByID(id)
    if err != nil { http.Error(w, "db error", http.StatusInternalServerError); return }
    if o == nil  { http.Error(w, "not found", http.StatusNotFound); return }
    writeJSON(w, http.StatusOK, o)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
    node := r.URL.Query().Get("node")
    if node == "" { http.Error(w, "node query param required", http.StatusBadRequest); return }
    orders, err := h.store.ListPendingByNode(node)
    if err != nil { http.Error(w, "db error", http.StatusInternalServerError); return }
    writeJSON(w, http.StatusOK, orders)
}
```

- [ ] **Run tests**

```bash
cd order-svc
TEST_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/trade_infra_test" \
go test ./internal/handler/... -v
```

Expected: 3 tests PASS.

- [ ] **Commit**

```bash
git add order-svc/internal/handler/
git commit -m "feat(order-svc): REST handler POST/GET /orders (TDD)"
```

---

### Task 12: order-svc LISTEN/NOTIFY listener

**Files:** `order-svc/internal/listener/listener.go`

- [ ] **Write listener.go**

Create `order-svc/internal/listener/listener.go`:

```go
package listener

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/lib/pq"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/evaluator"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

type TickPayload struct {
    Node string  `json:"node"`
    LMP  float64 `json:"lmp"`
}

type Listener struct {
    store    *order.Store
    db       *sql.DB
    dbURL    string
    FillsOut chan<- int64 // notifies callers of filled order IDs
}

func New(store *order.Store, db *sql.DB, dbURL string, fills chan<- int64) *Listener {
    return &Listener{store: store, db: db, dbURL: dbURL, FillsOut: fills}
}

func (l *Listener) Run() {
    onErr := func(ev pq.ListenerEventType, err error) {
        if err != nil {
            log.Printf("listener error: %v", err)
        }
    }
    pl := pq.NewListener(l.dbURL, 10*time.Second, time.Minute, onErr)
    if err := pl.Listen("price_ticks"); err != nil {
        log.Fatalf("LISTEN price_ticks: %v", err)
    }
    log.Println("order-svc: listening on price_ticks")
    for n := range pl.Notify {
        if n == nil { continue }
        var tick TickPayload
        if err := json.Unmarshal([]byte(n.Extra), &tick); err != nil {
            log.Printf("bad payload: %v", err)
            continue
        }
        l.evaluate(tick)
    }
}

func (l *Listener) evaluate(tick TickPayload) {
    orders, err := l.store.ListPendingByNode(tick.Node)
    if err != nil {
        log.Printf("list pending: %v", err)
        return
    }
    for _, o := range orders {
        if evaluator.ShouldFill(o, tick.LMP) {
            fa := tick.LMP
            if err := l.store.UpdateStatus(o.ID, order.StatusFilled, &fa); err != nil {
                log.Printf("update order %d: %v", o.ID, err)
                continue
            }
            l.notify(o.ID, tick.Node, tick.LMP, o.QuantityMW)
            if l.FillsOut != nil {
                l.FillsOut <- o.ID
            }
        }
    }
}

func (l *Listener) notify(orderID int64, node string, filledLMP, qty float64) {
    payload := fmt.Sprintf(`{"order_id":%d,"node":%q,"filled_lmp":%f,"quantity_mw":%f}`,
        orderID, node, filledLMP, qty)
    l.db.Exec("SELECT pg_notify('order_updates', $1)", payload)
}
```

- [ ] **Commit**

```bash
git add order-svc/internal/listener/listener.go
git commit -m "feat(order-svc): LISTEN price_ticks, evaluate orders, NOTIFY order_updates"
```

---

### Task 13: order-svc /metrics + main

**Files:** `order-svc/metrics/metrics.go`, `order-svc/cmd/server/main.go`

- [ ] **Write metrics.go**

Create `order-svc/metrics/metrics.go`:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    OrdersCreated = promauto.NewCounter(prometheus.CounterOpts{
        Name: "order_svc_orders_created_total",
        Help: "Total orders created",
    })
    OrdersFilled = promauto.NewCounter(prometheus.CounterOpts{
        Name: "order_svc_orders_filled_total",
        Help: "Total orders filled",
    })
    EvalLatency = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "order_svc_eval_latency_seconds",
        Help:    "Order evaluation latency",
        Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
    })
)
```

- [ ] **Write main.go**

Create `order-svc/cmd/server/main.go`:

```go
package main

import (
    "database/sql"
    "log"
    "net/http"
    "os"

    _ "github.com/lib/pq"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/handler"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/listener"
    "github.com/kangkabseok2021/trade_infra/order-svc/internal/order"
)

func getenv(k, d string) string {
    if v := os.Getenv(k); v != "" { return v }
    return d
}

func main() {
    dbURL      := getenv("DATABASE_URL",  "postgresql://postgres:postgres@localhost:5432/trade_infra")
    apiAddr    := getenv("API_ADDR",      ":8080")
    metricsAddr := getenv("METRICS_ADDR", ":9102")

    db, err := sql.Open("postgres", dbURL)
    if err != nil { log.Fatalf("db open: %v", err) }
    defer db.Close()

    store := order.NewStore(db)
    fills := make(chan int64, 100)
    l := listener.New(store, db, dbURL, fills)
    go l.Run()

    // REST API
    go func() {
        h := handler.New(store)
        mux := http.NewServeMux()
        mux.Handle("/orders", h)
        mux.Handle("/orders/", h)
        mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
            w.Write([]byte("ok"))
        })
        log.Printf("order-svc API listening on %s", apiAddr)
        log.Fatal(http.ListenAndServe(apiAddr, mux))
    }()

    // Prometheus metrics
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())
    log.Printf("order-svc metrics listening on %s", metricsAddr)
    log.Fatal(http.ListenAndServe(metricsAddr, mux))
}
```

- [ ] **Build and verify**

```bash
cd order-svc && go build ./...
```

Expected: no errors.

- [ ] **Commit**

```bash
git add order-svc/metrics/ order-svc/cmd/
git commit -m "feat(order-svc): Prometheus metrics and main server"
```

---

## Phase 3: risk-svc + Analytics + Grafana

### Task 14: risk-svc CMake + extern "C" header

**Files:** `risk-svc/CMakeLists.txt`, `risk-svc/include/riskcalc.h`

- [ ] **Write extern "C" header**

Create `risk-svc/include/riskcalc.h`:

```cpp
#pragma once
#ifdef __cplusplus
extern "C" {
#endif

/** Mark-to-market P&L: net_mw * (current_lmp - avg_fill_price). Units: USD. */
double calc_mtm_pnl(double net_mw, double avg_fill_price, double current_lmp);

/** Gross exposure: |net_mw| * current_lmp. Units: USD/h equivalent. */
double calc_net_exposure(double net_mw, double current_lmp);

/** Returns 1 if |net_exposure_mw| exceeds position_limit_mw, else 0. */
int check_limit_breach(double net_exposure_mw, double position_limit_mw);

#ifdef __cplusplus
}
#endif
```

- [ ] **Write CMakeLists.txt**

Create `risk-svc/CMakeLists.txt`:

```cmake
cmake_minimum_required(VERSION 3.20)
project(risk_svc CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

include(FetchContent)
FetchContent_Declare(googletest
    GIT_REPOSITORY https://github.com/google/googletest.git
    GIT_TAG        v1.14.0)
set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

add_library(riskcalc SHARED src/risk_calc.cpp)
target_include_directories(riskcalc PUBLIC include PRIVATE src)
if(APPLE)
    target_link_options(riskcalc PRIVATE -undefined dynamic_lookup)
endif()

enable_testing()
add_executable(test_riskcalc tests/test_risk_calc.cpp)
target_link_libraries(test_riskcalc riskcalc GTest::gtest_main)
include(GoogleTest)
gtest_discover_tests(test_riskcalc)
```

- [ ] **Commit**

```bash
git add risk-svc/CMakeLists.txt risk-svc/include/riskcalc.h
git commit -m "feat(risk-svc): CMake structure and extern-C ABI header"
```

---

### Task 15: risk calc — failing tests

**Files:** `risk-svc/tests/test_risk_calc.cpp`

- [ ] **Write failing tests**

Create `risk-svc/tests/test_risk_calc.cpp`:

```cpp
#include <gtest/gtest.h>
#include "riskcalc.h"

TEST(RiskCalcTest, MtmPnlLongPositionProfitable) {
    // Long 10 MW, avg fill $40, current $45 → profit = 10 * 5 = $50
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(10.0, 40.0, 45.0), 50.0);
}

TEST(RiskCalcTest, MtmPnlLongPositionLoss) {
    // Long 10 MW, avg fill $40, current $35 → loss = 10 * -5 = -$50
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(10.0, 40.0, 35.0), -50.0);
}

TEST(RiskCalcTest, MtmPnlShortPosition) {
    // Short -10 MW, avg fill $40, current $35 → profit = -10 * -5 = $50
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(-10.0, 40.0, 35.0), 50.0);
}

TEST(RiskCalcTest, MtmPnlFlatPosition) {
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(0.0, 40.0, 45.0), 0.0);
}

TEST(RiskCalcTest, NetExposureLong) {
    // 10 MW long at $45 → $450 exposure
    EXPECT_DOUBLE_EQ(calc_net_exposure(10.0, 45.0), 450.0);
}

TEST(RiskCalcTest, NetExposureShort) {
    // -10 MW short at $45 → $450 exposure (absolute)
    EXPECT_DOUBLE_EQ(calc_net_exposure(-10.0, 45.0), 450.0);
}

TEST(RiskCalcTest, LimitBreachExceeded) {
    EXPECT_EQ(check_limit_breach(15.0, 10.0), 1);
}

TEST(RiskCalcTest, LimitBreachNotExceeded) {
    EXPECT_EQ(check_limit_breach(8.0, 10.0), 0);
}

TEST(RiskCalcTest, LimitBreachAtExactLimit) {
    EXPECT_EQ(check_limit_breach(10.0, 10.0), 0);
}
```

- [ ] **Verify build fails**

```bash
cd risk-svc && cmake -B build -S . && cmake --build build --target test_riskcalc 2>&1 | tail -5
```

Expected: linker error — missing `calc_mtm_pnl` symbols.

- [ ] **Commit failing tests**

```bash
git add risk-svc/tests/test_risk_calc.cpp
git commit -m "test(risk-svc): GoogleTest suite for risk calculations (failing)"
```

---

### Task 16: risk calc — implementation

**Files:** `risk-svc/src/risk_calc.h`, `risk-svc/src/risk_calc.cpp`

- [ ] **Write risk_calc.h**

Create `risk-svc/src/risk_calc.h`:

```cpp
#pragma once

class RiskCalc {
public:
    static double mtm_pnl(double net_mw, double avg_fill_price, double current_lmp);
    static double net_exposure(double net_mw, double current_lmp);
    static bool   limit_breach(double net_exposure_mw, double position_limit_mw);
};
```

- [ ] **Write risk_calc.cpp**

Create `risk-svc/src/risk_calc.cpp`:

```cpp
#include "risk_calc.h"
#include "riskcalc.h"
#include <cmath>

double RiskCalc::mtm_pnl(double net_mw, double avg_fill_price, double current_lmp) {
    return net_mw * (current_lmp - avg_fill_price);
}

double RiskCalc::net_exposure(double net_mw, double current_lmp) {
    return std::abs(net_mw) * current_lmp;
}

bool RiskCalc::limit_breach(double net_exposure_mw, double position_limit_mw) {
    return net_exposure_mw > position_limit_mw;
}

extern "C" {
double calc_mtm_pnl(double net_mw, double avg_fill_price, double current_lmp) {
    return RiskCalc::mtm_pnl(net_mw, avg_fill_price, current_lmp);
}
double calc_net_exposure(double net_mw, double current_lmp) {
    return RiskCalc::net_exposure(net_mw, current_lmp);
}
int check_limit_breach(double net_exposure_mw, double position_limit_mw) {
    return RiskCalc::limit_breach(net_exposure_mw, position_limit_mw) ? 1 : 0;
}
}
```

- [ ] **Build and run tests**

```bash
cd risk-svc
cmake -B build -S . && cmake --build build --target test_riskcalc
cd build && ctest --output-on-failure
```

Expected: `9/9 tests passed`.

- [ ] **Commit**

```bash
git add risk-svc/src/
git commit -m "feat(risk-svc): risk calculations — MTM P&L, exposure, limit breach"
```

---

### Task 17: risk-svc Go module + CGo wrapper (TDD)

**Files:** `risk-svc/go.mod`, `risk-svc/internal/riskcalc/riskcalc.go`, `risk-svc/internal/riskcalc/riskcalc_test.go`

- [ ] **Initialize module**

```bash
cd risk-svc
go mod init github.com/kangkabseok2021/trade_infra/risk-svc
go get github.com/lib/pq@v1.10.9
go get github.com/prometheus/client_golang@v1.19.0
```

- [ ] **Build libriskcalc.so first**

```bash
cd risk-svc && cmake -B build -S . && cmake --build build --target riskcalc
ls build/libriskcalc.*
```

Expected: `libriskcalc.dylib` (macOS) or `libriskcalc.so` (Linux).

- [ ] **Write failing CGo test**

Create `risk-svc/internal/riskcalc/riskcalc_test.go`:

```go
package riskcalc_test

import (
    "testing"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/riskcalc"
)

func TestMtmPnlLong(t *testing.T) {
    got := riskcalc.CalcMtmPnl(10.0, 40.0, 45.0)
    if got != 50.0 { t.Errorf("want 50.0 got %f", got) }
}

func TestNetExposureAbsolute(t *testing.T) {
    got := riskcalc.CalcNetExposure(-10.0, 45.0)
    if got != 450.0 { t.Errorf("want 450.0 got %f", got) }
}

func TestLimitBreach(t *testing.T) {
    if !riskcalc.CheckLimitBreach(15.0, 10.0) { t.Error("15 > 10 should breach") }
    if riskcalc.CheckLimitBreach(8.0, 10.0)   { t.Error("8 <= 10 should not breach") }
}
```

- [ ] **Run to verify failure**

```bash
cd risk-svc && go test ./internal/riskcalc/... 2>&1 | head -5
```

Expected: compile error — `riskcalc.go` missing.

- [ ] **Implement CGo wrapper**

Create `risk-svc/internal/riskcalc/riskcalc.go`:

```go
package riskcalc

/*
#cgo CFLAGS: -I${SRCDIR}/../../../risk-svc/include
#cgo linux  LDFLAGS: -L${SRCDIR}/../../../risk-svc/build -lriskcalc -Wl,-rpath,${SRCDIR}/../../../risk-svc/build
#cgo darwin LDFLAGS: -L${SRCDIR}/../../../risk-svc/build -lriskcalc
#include "riskcalc.h"
*/
import "C"

func CalcMtmPnl(netMW, avgFillPrice, currentLMP float64) float64 {
    return float64(C.calc_mtm_pnl(C.double(netMW), C.double(avgFillPrice), C.double(currentLMP)))
}

func CalcNetExposure(netMW, currentLMP float64) float64 {
    return float64(C.calc_net_exposure(C.double(netMW), C.double(currentLMP)))
}

func CheckLimitBreach(netExposureMW, positionLimitMW float64) bool {
    return C.check_limit_breach(C.double(netExposureMW), C.double(positionLimitMW)) == 1
}
```

- [ ] **Run tests**

```bash
cd risk-svc && go test ./internal/riskcalc/... -v
```

Expected: 3 tests PASS.

- [ ] **Commit**

```bash
git add risk-svc/go.mod risk-svc/go.sum risk-svc/internal/riskcalc/
git commit -m "feat(risk-svc): Go module + CGo wrapper for libriskcalc.so (TDD)"
```

---

### Task 18: risk-svc model + store (TDD)

**Files:** `risk-svc/internal/risk/model.go`, `risk-svc/internal/risk/store.go`, `risk-svc/internal/risk/store_test.go`

- [ ] **Write model.go**

Create `risk-svc/internal/risk/model.go`:

```go
package risk

import "time"

type Snapshot struct {
    ID             int64     `json:"id"`
    Node           string    `json:"node"`
    MtmPnl         float64   `json:"mtm_pnl"`
    NetExposureMW  float64   `json:"net_exposure_mw"`
    LimitHeadroom  float64   `json:"limit_headroom"`
    SnapshotAt     time.Time `json:"snapshot_at"`
}

type Position struct {
    Node      string   `json:"node"`
    NetMW     float64  `json:"net_mw"`
    AvgPrice  *float64 `json:"avg_price,omitempty"`
}
```

- [ ] **Write failing store tests**

Create `risk-svc/internal/risk/store_test.go`:

```go
package risk_test

import (
    "database/sql"
    "os"
    "testing"

    _ "github.com/lib/pq"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
)

func testDB(t *testing.T) *sql.DB {
    t.Helper()
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" { url = "postgresql://postgres:postgres@localhost:5432/trade_infra_test" }
    db, err := sql.Open("postgres", url)
    if err != nil { t.Fatalf("db: %v", err) }
    t.Cleanup(func() {
        db.Exec("DELETE FROM risk_snapshots")
        db.Exec("DELETE FROM positions")
        db.Close()
    })
    return db
}

func TestStore_SaveSnapshot(t *testing.T) {
    s := risk.NewStore(testDB(t))
    snap := &risk.Snapshot{Node: "HB_NORTH", MtmPnl: 125.50, NetExposureMW: 10, LimitHeadroom: 40}
    if err := s.SaveSnapshot(snap); err != nil { t.Fatalf("save: %v", err) }
    if snap.ID == 0 { t.Error("expected ID") }
}

func TestStore_LatestSnapshot(t *testing.T) {
    s := risk.NewStore(testDB(t))
    s.SaveSnapshot(&risk.Snapshot{Node: "HB_NORTH", MtmPnl: 100, NetExposureMW: 10, LimitHeadroom: 40})
    s.SaveSnapshot(&risk.Snapshot{Node: "HB_NORTH", MtmPnl: 200, NetExposureMW: 12, LimitHeadroom: 38})
    snap, err := s.LatestSnapshot("HB_NORTH")
    if err != nil { t.Fatalf("latest: %v", err) }
    if snap.MtmPnl != 200 { t.Errorf("want 200 got %f", snap.MtmPnl) }
}

func TestStore_UpsertPosition(t *testing.T) {
    s := risk.NewStore(testDB(t))
    avg := 45.0
    if err := s.UpsertPosition("HB_NORTH", 10.0, &avg); err != nil { t.Fatalf("upsert: %v", err) }
    pos, err := s.GetPosition("HB_NORTH")
    if err != nil { t.Fatalf("get: %v", err) }
    if pos.NetMW != 10.0 { t.Errorf("want 10 got %f", pos.NetMW) }
}
```

- [ ] **Run to verify failure**

```bash
cd risk-svc && go test ./internal/risk/... 2>&1 | head -5
```

Expected: compile error.

- [ ] **Implement store.go**

Create `risk-svc/internal/risk/store.go`:

```go
package risk

import "database/sql"

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) SaveSnapshot(snap *Snapshot) error {
    return s.db.QueryRow(`
        INSERT INTO risk_snapshots (node, mtm_pnl, net_exposure_mw, limit_headroom)
        VALUES ($1,$2,$3,$4) RETURNING id,snapshot_at`,
        snap.Node, snap.MtmPnl, snap.NetExposureMW, snap.LimitHeadroom,
    ).Scan(&snap.ID, &snap.SnapshotAt)
}

func (s *Store) LatestSnapshot(node string) (*Snapshot, error) {
    var snap Snapshot
    err := s.db.QueryRow(`
        SELECT id,node,mtm_pnl,net_exposure_mw,limit_headroom,snapshot_at
        FROM risk_snapshots WHERE node=$1 ORDER BY snapshot_at DESC LIMIT 1`, node,
    ).Scan(&snap.ID, &snap.Node, &snap.MtmPnl, &snap.NetExposureMW, &snap.LimitHeadroom, &snap.SnapshotAt)
    if err == sql.ErrNoRows { return nil, nil }
    return &snap, err
}

func (s *Store) UpsertPosition(node string, netMW float64, avgPrice *float64) error {
    _, err := s.db.Exec(`
        INSERT INTO positions (node, net_mw, avg_price)
        VALUES ($1,$2,$3)
        ON CONFLICT (node) DO UPDATE SET net_mw=$2, avg_price=$3, updated_at=now()`,
        node, netMW, avgPrice)
    return err
}

func (s *Store) GetPosition(node string) (*Position, error) {
    var p Position
    err := s.db.QueryRow(`SELECT node,net_mw,avg_price FROM positions WHERE node=$1`, node).
        Scan(&p.Node, &p.NetMW, &p.AvgPrice)
    if err == sql.ErrNoRows { return nil, nil }
    return &p, err
}
```

- [ ] **Run tests**

```bash
cd risk-svc
TEST_DATABASE_URL="postgresql://postgres:postgres@localhost:5432/trade_infra_test" \
go test ./internal/risk/... -v
```

Expected: 3 tests PASS.

- [ ] **Commit**

```bash
git add risk-svc/internal/risk/
git commit -m "feat(risk-svc): risk model and PostgreSQL store (TDD)"
```

---

### Task 19: risk-svc listener + REST API + main

**Files:** `risk-svc/internal/listener/listener.go`, `risk-svc/internal/handler/handler.go`, `risk-svc/metrics/metrics.go`, `risk-svc/cmd/server/main.go`

- [ ] **Write listener.go**

Create `risk-svc/internal/listener/listener.go`:

```go
package listener

import (
    "database/sql"
    "encoding/json"
    "log"
    "time"

    "github.com/lib/pq"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/riskcalc"
)

const positionLimitMW = 50.0

type FillPayload struct {
    OrderID    int64   `json:"order_id"`
    Node       string  `json:"node"`
    FilledLMP  float64 `json:"filled_lmp"`
    QuantityMW float64 `json:"quantity_mw"`
}

type Listener struct {
    store *risk.Store
    db    *sql.DB
    dbURL string
}

func New(store *risk.Store, db *sql.DB, dbURL string) *Listener {
    return &Listener{store: store, db: db, dbURL: dbURL}
}

func (l *Listener) Run() {
    onErr := func(_ pq.ListenerEventType, err error) {
        if err != nil { log.Printf("risk listener error: %v", err) }
    }
    pl := pq.NewListener(l.dbURL, 10*time.Second, time.Minute, onErr)
    if err := pl.Listen("order_updates"); err != nil {
        log.Fatalf("LISTEN order_updates: %v", err)
    }
    log.Println("risk-svc: listening on order_updates")
    for n := range pl.Notify {
        if n == nil { continue }
        var fill FillPayload
        if err := json.Unmarshal([]byte(n.Extra), &fill); err != nil {
            log.Printf("bad payload: %v", err)
            continue
        }
        l.process(fill)
    }
}

func (l *Listener) process(fill FillPayload) {
    pos, _ := l.store.GetPosition(fill.Node)
    netMW := fill.QuantityMW
    avgPrice := fill.FilledLMP
    if pos != nil {
        netMW += pos.NetMW
        if pos.AvgPrice != nil {
            avgPrice = ((*pos.AvgPrice * pos.NetMW) + (fill.FilledLMP * fill.QuantityMW)) / netMW
        }
    }
    l.store.UpsertPosition(fill.Node, netMW, &avgPrice)

    mtmPnl     := riskcalc.CalcMtmPnl(netMW, avgPrice, fill.FilledLMP)
    netExp      := riskcalc.CalcNetExposure(netMW, fill.FilledLMP)
    headroom    := positionLimitMW - netExp

    snap := &risk.Snapshot{
        Node: fill.Node, MtmPnl: mtmPnl,
        NetExposureMW: netExp, LimitHeadroom: headroom,
    }
    if err := l.store.SaveSnapshot(snap); err != nil {
        log.Printf("save snapshot: %v", err)
    }
    if riskcalc.CheckLimitBreach(netExp, positionLimitMW) {
        log.Printf("LIMIT BREACH: node=%s net_exposure=%.2f limit=%.2f", fill.Node, netExp, positionLimitMW)
    }
}
```

- [ ] **Write handler.go**

Create `risk-svc/internal/handler/handler.go`:

```go
package handler

import (
    "encoding/json"
    "net/http"

    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
)

type Handler struct {
    store *risk.Store
    mux   *http.ServeMux
}

func New(store *risk.Store) *Handler {
    h := &Handler{store: store, mux: http.NewServeMux()}
    h.mux.HandleFunc("GET /risk/{node}", h.getSnapshot)
    return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

func (h *Handler) getSnapshot(w http.ResponseWriter, r *http.Request) {
    node := r.PathValue("node")
    snap, err := h.store.LatestSnapshot(node)
    if err != nil { http.Error(w, "db error", 500); return }
    if snap == nil { http.Error(w, "no snapshot", 404); return }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(snap)
}
```

- [ ] **Write metrics.go**

Create `risk-svc/metrics/metrics.go`:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    SnapshotsSaved = promauto.NewCounter(prometheus.CounterOpts{
        Name: "risk_svc_snapshots_saved_total",
        Help: "Total risk snapshots written",
    })
    LimitBreaches = promauto.NewCounter(prometheus.CounterOpts{
        Name: "risk_svc_limit_breaches_total",
        Help: "Total position limit breaches detected",
    })
)
```

- [ ] **Write main.go**

Create `risk-svc/cmd/server/main.go`:

```go
package main

import (
    "database/sql"
    "log"
    "net/http"
    "os"

    _ "github.com/lib/pq"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/handler"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/listener"
    "github.com/kangkabseok2021/trade_infra/risk-svc/internal/risk"
)

func getenv(k, d string) string {
    if v := os.Getenv(k); v != "" { return v }
    return d
}

func main() {
    dbURL       := getenv("DATABASE_URL",  "postgresql://postgres:postgres@localhost:5432/trade_infra")
    apiAddr     := getenv("API_ADDR",      ":8081")
    metricsAddr := getenv("METRICS_ADDR",  ":9103")

    db, err := sql.Open("postgres", dbURL)
    if err != nil { log.Fatalf("db: %v", err) }
    defer db.Close()

    store := risk.NewStore(db)
    l := listener.New(store, db, dbURL)
    go l.Run()

    go func() {
        h := handler.New(store)
        mux := http.NewServeMux()
        mux.Handle("/risk/", h)
        mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
        log.Printf("risk-svc API on %s", apiAddr)
        log.Fatal(http.ListenAndServe(apiAddr, mux))
    }()

    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())
    log.Printf("risk-svc metrics on %s", metricsAddr)
    log.Fatal(http.ListenAndServe(metricsAddr, mux))
}
```

- [ ] **Build**

```bash
cd risk-svc && go build ./...
```

Expected: no errors.

- [ ] **Commit**

```bash
git add risk-svc/internal/listener/ risk-svc/internal/handler/ risk-svc/metrics/ risk-svc/cmd/
git commit -m "feat(risk-svc): order_updates listener, REST API, Prometheus metrics, main"
```

---

### Task 20: analytics.py (TDD)

**Files:** `python/tests/test_analytics.py`, `python/analytics.py`

- [ ] **Write failing tests**

Create `python/tests/test_analytics.py`:

```python
import sys, os
sys.path.insert(0, os.path.dirname(os.path.dirname(__file__)))
from analytics import format_pnl_report, format_position_summary

def test_pnl_report_positive():
    result = format_pnl_report("HB_NORTH", mtm_pnl=125.50, net_exposure_mw=10.0, limit_headroom=40.0)
    assert "HB_NORTH" in result
    assert "125.50" in result
    assert "WITHIN LIMITS" in result

def test_pnl_report_breach():
    result = format_pnl_report("HB_SOUTH", mtm_pnl=-50.0, net_exposure_mw=55.0, limit_headroom=-5.0)
    assert "BREACH" in result

def test_position_summary_long():
    result = format_position_summary("HB_NORTH", net_mw=10.0, avg_price=45.0)
    assert "LONG" in result
    assert "10.0" in result

def test_position_summary_short():
    result = format_position_summary("HB_NORTH", net_mw=-5.0, avg_price=42.0)
    assert "SHORT" in result

def test_position_summary_flat():
    result = format_position_summary("HB_NORTH", net_mw=0.0, avg_price=None)
    assert "FLAT" in result
```

- [ ] **Run to verify failure**

```bash
cd python && uv run pytest tests/test_analytics.py -v 2>&1 | head -5
```

Expected: `ModuleNotFoundError: No module named 'analytics'`

- [ ] **Implement analytics.py**

Create `python/analytics.py`:

```python
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
```

- [ ] **Run tests**

```bash
cd python && uv run pytest tests/test_analytics.py -v
```

Expected: 5 tests PASS.

- [ ] **Commit**

```bash
git add python/analytics.py python/tests/test_analytics.py
git commit -m "feat(python): analytics.py — P&L report, position summary (TDD)"
```

---

### Task 21: Grafana dashboard JSON

**Files:** `infra/grafana/dashboards/trade_infra.json`

- [ ] **Write dashboard JSON**

Create `infra/grafana/dashboards/trade_infra.json`:

```json
{
  "__inputs": [],
  "__requires": [{ "type": "grafana", "id": "grafana", "name": "Grafana", "version": "10.0.0" }],
  "title": "trade_infra",
  "uid": "trade-infra-main",
  "version": 1,
  "refresh": "10s",
  "time": { "from": "now-1h", "to": "now" },
  "panels": [
    {
      "id": 1, "type": "stat", "title": "Tick Ingestion Rate",
      "gridPos": { "x": 0, "y": 0, "w": 6, "h": 4 },
      "targets": [{
        "expr": "rate(market_data_tick_total[1m])",
        "legendFormat": "ticks/s"
      }]
    },
    {
      "id": 2, "type": "stat", "title": "Orders Filled (total)",
      "gridPos": { "x": 6, "y": 0, "w": 6, "h": 4 },
      "targets": [{
        "expr": "order_svc_orders_filled_total",
        "legendFormat": "fills"
      }]
    },
    {
      "id": 3, "type": "stat", "title": "Limit Breaches (total)",
      "gridPos": { "x": 12, "y": 0, "w": 6, "h": 4 },
      "targets": [{
        "expr": "risk_svc_limit_breaches_total",
        "legendFormat": "breaches"
      }]
    },
    {
      "id": 4, "type": "timeseries", "title": "Order Evaluation Latency (p99)",
      "gridPos": { "x": 0, "y": 4, "w": 12, "h": 8 },
      "targets": [{
        "expr": "histogram_quantile(0.99, rate(order_svc_eval_latency_seconds_bucket[5m]))",
        "legendFormat": "p99"
      }]
    },
    {
      "id": 5, "type": "timeseries", "title": "Service Availability",
      "gridPos": { "x": 12, "y": 4, "w": 12, "h": 8 },
      "targets": [
        { "expr": "up{job=\"market-data-svc\"}", "legendFormat": "market-data-svc" },
        { "expr": "up{job=\"order-svc\"}",       "legendFormat": "order-svc" },
        { "expr": "up{job=\"risk-svc\"}",        "legendFormat": "risk-svc" }
      ]
    }
  ]
}
```

- [ ] **Commit**

```bash
git add infra/grafana/dashboards/trade_infra.json
git commit -m "feat(infra): Grafana dashboard — ticks, fills, latency, availability"
```

---

## Phase 4: Docker Compose + CI/CD

### Task 22: Docker Compose + Prometheus config

**Files:** `infra/docker-compose.yml`, `infra/prometheus.yml`, `infra/slo_rules.yml`

- [ ] **Write docker-compose.yml**

Create `infra/docker-compose.yml`:

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: trade_infra
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      retries: 10
    volumes:
      - ../sql/schema.sql:/docker-entrypoint-initdb.d/schema.sql

  market-data-svc:
    build: { context: ../market-data-svc, dockerfile: Dockerfile }
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra
      NODE_NAME: HB_NORTH
      BASE_LMP: "45.0"
      VOLATILITY: "5.0"
      INTERVAL_MS: "1000"
      METRICS_PORT: "9101"
    ports: ["9101:9101"]
    depends_on:
      postgres: { condition: service_healthy }

  order-svc:
    build: { context: ../order-svc, dockerfile: Dockerfile }
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra
      API_ADDR: ":8080"
      METRICS_ADDR: ":9102"
    ports: ["8080:8080", "9102:9102"]
    depends_on:
      postgres: { condition: service_healthy }

  risk-svc:
    build: { context: ../risk-svc, dockerfile: Dockerfile }
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/trade_infra
      API_ADDR: ":8081"
      METRICS_ADDR: ":9103"
    ports: ["8081:8081", "9103:9103"]
    depends_on:
      postgres: { condition: service_healthy }

  prometheus:
    image: prom/prometheus:v2.51.0
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - ./slo_rules.yml:/etc/prometheus/slo_rules.yml
    ports: ["9090:9090"]
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --web.enable-lifecycle

  grafana:
    image: grafana/grafana:10.4.0
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
      GF_AUTH_ANONYMOUS_ENABLED: "true"
    volumes:
      - ./grafana/dashboards:/var/lib/grafana/dashboards
    ports: ["3000:3000"]
    depends_on: [prometheus]
```

- [ ] **Write prometheus.yml**

Create `infra/prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - slo_rules.yml

scrape_configs:
  - job_name: market-data-svc
    static_configs:
      - targets: ["market-data-svc:9101"]

  - job_name: order-svc
    static_configs:
      - targets: ["order-svc:9102"]

  - job_name: risk-svc
    static_configs:
      - targets: ["risk-svc:9103"]
```

- [ ] **Write slo_rules.yml**

Create `infra/slo_rules.yml`:

```yaml
groups:
  - name: trade_infra_slos
    rules:
      # SLO: order evaluation p99 latency < 50ms
      - record: slo:order_eval_latency_p99:5m
        expr: histogram_quantile(0.99, rate(order_svc_eval_latency_seconds_bucket[5m]))

      # SLO: service availability (1 = up)
      - record: slo:service_availability:5m
        expr: avg_over_time(up[5m])

      # Alert: p99 latency breach
      - alert: OrderEvalLatencyHigh
        expr: slo:order_eval_latency_p99:5m > 0.05
        for: 2m
        labels: { severity: warning }
        annotations:
          summary: "order-svc p99 eval latency {{ $value | humanizeDuration }} > 50ms"

      # Alert: service down
      - alert: ServiceDown
        expr: up == 0
        for: 1m
        labels: { severity: critical }
        annotations:
          summary: "{{ $labels.job }} is down"
```

- [ ] **Commit**

```bash
git add infra/docker-compose.yml infra/prometheus.yml infra/slo_rules.yml
git commit -m "feat(infra): Docker Compose, Prometheus scrape config, SLO alerting rules"
```

---

### Task 23: GitHub Actions — market-data-svc CI

**Files:** `.github/workflows/market-data-svc.yml`

- [ ] **Write workflow**

Create `.github/workflows/market-data-svc.yml`:

```yaml
name: market-data-svc

on:
  push:
    paths: ["market-data-svc/**"]
  pull_request:
    paths: ["market-data-svc/**"]

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install libpq
        run: sudo apt-get install -y libpq-dev

      - name: Configure CMake
        run: cmake -B market-data-svc/build -S market-data-svc

      - name: Build
        run: cmake --build market-data-svc/build --target market_data_svc test_marketdata -j4

      - name: Run tests
        run: |
          cd market-data-svc/build
          ctest --output-on-failure
```

- [ ] **Commit**

```bash
git add .github/workflows/market-data-svc.yml
git commit -m "ci: GitHub Actions for market-data-svc (CMake + GoogleTest)"
```

---

### Task 24: GitHub Actions — order-svc and risk-svc CI

**Files:** `.github/workflows/order-svc.yml`, `.github/workflows/risk-svc.yml`

- [ ] **Write order-svc.yml**

Create `.github/workflows/order-svc.yml`:

```yaml
name: order-svc

on:
  push:
    paths: ["order-svc/**"]
  pull_request:
    paths: ["order-svc/**"]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: trade_infra_test
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-retries 10
        ports: ["5432:5432"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }

      - name: Apply schema
        run: psql postgresql://postgres:postgres@localhost:5432/trade_infra_test -f sql/schema.sql

      - name: Run tests
        working-directory: order-svc
        env:
          TEST_DATABASE_URL: postgresql://postgres:postgres@localhost:5432/trade_infra_test
        run: go test ./... -v
```

- [ ] **Write risk-svc.yml**

Create `.github/workflows/risk-svc.yml`:

```yaml
name: risk-svc

on:
  push:
    paths: ["risk-svc/**"]
  pull_request:
    paths: ["risk-svc/**"]

jobs:
  cpp-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: cmake -B risk-svc/build -S risk-svc
      - run: cmake --build risk-svc/build --target test_riskcalc -j4
      - run: cd risk-svc/build && ctest --output-on-failure

  go-tests:
    runs-on: ubuntu-latest
    needs: cpp-tests
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: trade_infra_test
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-retries 10
        ports: ["5432:5432"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - name: Build libriskcalc.so
        run: cmake -B risk-svc/build -S risk-svc && cmake --build risk-svc/build --target riskcalc
      - name: Apply schema
        run: psql postgresql://postgres:postgres@localhost:5432/trade_infra_test -f sql/schema.sql
      - name: Run Go tests
        working-directory: risk-svc
        env:
          TEST_DATABASE_URL: postgresql://postgres:postgres@localhost:5432/trade_infra_test
        run: go test ./... -v
```

- [ ] **Commit**

```bash
git add .github/workflows/order-svc.yml .github/workflows/risk-svc.yml
git commit -m "ci: GitHub Actions for order-svc and risk-svc"
```

---

### Task 25: Python CI + integration smoke test + integration workflow

**Files:** `.github/workflows/python.yml`, `python/smoke_test.py`, `.github/workflows/integration.yml`

- [ ] **Write python.yml**

Create `.github/workflows/python.yml`:

```yaml
name: python

on:
  push:
    paths: ["python/**"]
  pull_request:
    paths: ["python/**"]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
      - name: Lint
        run: cd python && uv run ruff check .
      - name: Test
        run: cd python && uv run pytest tests/ -v
```

- [ ] **Write smoke_test.py**

Create `python/smoke_test.py`:

```python
"""Integration smoke test: runs against a live docker-compose stack."""
import time
import sys
import requests
import psycopg2

ORDER_SVC = "http://localhost:8080"
RISK_SVC  = "http://localhost:8081"
DB_URL    = "postgresql://postgres:postgres@localhost:5432/trade_infra"

def wait_for(url: str, timeout: int = 60) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            r = requests.get(url, timeout=2)
            if r.status_code == 200:
                return
        except Exception:
            pass
        time.sleep(2)
    raise TimeoutError(f"{url} not healthy after {timeout}s")

def main() -> int:
    print("Waiting for services...")
    wait_for(f"{ORDER_SVC}/health")
    wait_for(f"{RISK_SVC}/health")
    print("Services healthy.")

    # Wait for some price ticks to accumulate
    time.sleep(5)
    conn = psycopg2.connect(DB_URL)
    cur = conn.cursor()
    cur.execute("SELECT COUNT(*) FROM price_ticks")
    tick_count = cur.fetchone()[0]
    print(f"Tick count: {tick_count}")
    assert tick_count > 0, "No price ticks — market-data-svc not running"

    # Get current LMP for HB_NORTH
    cur.execute("SELECT lmp FROM price_ticks WHERE node='HB_NORTH' ORDER BY timestamp DESC LIMIT 1")
    row = cur.fetchone()
    assert row is not None, "No HB_NORTH ticks"
    current_lmp = float(row[0])
    print(f"Current HB_NORTH LMP: ${current_lmp:.4f}")

    # Submit a BUY order with limit above current LMP (should fill quickly)
    limit_price = current_lmp + 10.0
    resp = requests.post(f"{ORDER_SVC}/orders", json={
        "node": "HB_NORTH", "side": "BUY",
        "quantity_mw": 5.0, "limit_price": limit_price,
    })
    assert resp.status_code == 201, f"Create order failed: {resp.status_code} {resp.text}"
    order_id = resp.json()["id"]
    print(f"Created order {order_id} limit={limit_price:.4f}")

    # Wait for fill
    deadline = time.time() + 30
    filled = False
    while time.time() < deadline:
        r = requests.get(f"{ORDER_SVC}/orders/{order_id}")
        if r.json().get("status") == "FILLED":
            filled = True
            break
        time.sleep(2)
    assert filled, f"Order {order_id} not filled within 30s"
    print(f"Order {order_id} filled.")

    # Verify risk snapshot exists
    time.sleep(3)
    cur.execute("SELECT COUNT(*) FROM risk_snapshots WHERE node='HB_NORTH'")
    snap_count = cur.fetchone()[0]
    assert snap_count > 0, "No risk snapshot created after fill"
    print(f"Risk snapshots for HB_NORTH: {snap_count}")

    conn.close()
    print("\nSmoke test PASSED.")
    return 0

if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Write integration.yml**

Create `.github/workflows/integration.yml`:

```yaml
name: integration

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }

      - name: Install libpq
        run: sudo apt-get install -y libpq-dev

      - name: Build C++ libraries
        run: |
          cmake -B market-data-svc/build -S market-data-svc
          cmake --build market-data-svc/build --target marketdata market_data_svc -j4
          cmake -B risk-svc/build -S risk-svc
          cmake --build risk-svc/build --target riskcalc -j4

      - name: Build Go services
        run: |
          cd order-svc && go build ./cmd/server
          cd ../risk-svc && go build ./cmd/server

      - name: Start stack
        working-directory: infra
        run: docker compose up -d --wait

      - name: Run smoke test
        run: |
          cd python && uv sync --extra dev
          uv run python smoke_test.py

      - name: Dump logs on failure
        if: failure()
        working-directory: infra
        run: docker compose logs

      - name: Tear down
        if: always()
        working-directory: infra
        run: docker compose down -v
```

- [ ] **Commit**

```bash
git add .github/workflows/python.yml python/smoke_test.py .github/workflows/integration.yml
git commit -m "ci: Python lint/test, integration smoke test, full pipeline workflow"
```

---

## Spec Coverage Check

| Spec requirement | Covered by |
|---|---|
| market-data-svc C++ shared lib + extern "C" | Tasks 4–7 |
| order-svc Go bid lifecycle | Tasks 8–13 |
| risk-svc C++ P&L calcs + Go CGo wrapper | Tasks 14–19 |
| Python data generator (uv) | Task 3 |
| Python analytics CLI | Task 20 |
| PostgreSQL schema (all 4 tables) | Task 2 |
| LISTEN/NOTIFY price_ticks → order eval | Task 12 |
| LISTEN/NOTIFY order_updates → risk snapshot | Task 19 |
| Prometheus /metrics on all services | Tasks 7, 13, 19 |
| Grafana dashboard | Task 21 |
| Docker Compose with health checks | Task 22 |
| SLO Prometheus rules + alerts | Task 22 |
| GitHub Actions per-service CI | Tasks 23–24 |
| GitHub Actions integration test | Task 25 |
| Integration smoke test | Task 25 |
| Rust migration path (clean extern "C" ABI) | Tasks 4, 14 |
