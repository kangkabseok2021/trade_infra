#pragma once

class RiskCalc {
public:
    static double mtm_pnl(double net_mw, double avg_fill_price, double current_lmp);
    static double net_exposure(double net_mw, double current_lmp);
    static bool   limit_breach(double net_exposure_mw, double position_limit_mw);
};
