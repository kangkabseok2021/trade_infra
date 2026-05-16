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
