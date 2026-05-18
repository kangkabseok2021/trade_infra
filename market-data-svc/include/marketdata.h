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
