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
