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
