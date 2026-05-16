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
