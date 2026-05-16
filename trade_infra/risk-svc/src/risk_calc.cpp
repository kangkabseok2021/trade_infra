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
