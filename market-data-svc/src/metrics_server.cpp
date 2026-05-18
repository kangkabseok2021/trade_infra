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
