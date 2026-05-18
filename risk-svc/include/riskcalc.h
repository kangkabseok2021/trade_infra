#pragma once
#ifdef __cplusplus
extern "C" {
#endif

/** Mark-to-market P&L: net_mw * (current_lmp - avg_fill_price). Units: USD. */
double calc_mtm_pnl(double net_mw, double avg_fill_price, double current_lmp);

/** Net position in MW: |net_mw|. current_lmp reserved for a future USD-exposure variant. */
double calc_net_exposure(double net_mw, double current_lmp);

/** Returns 1 if |net_exposure_mw| exceeds position_limit_mw, else 0. */
int check_limit_breach(double net_exposure_mw, double position_limit_mw);

#ifdef __cplusplus
}
#endif
