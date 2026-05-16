use std::os::raw::c_int;

#[no_mangle]
pub extern "C" fn calc_mtm_pnl(
    net_mw: f64,
    avg_fill_price: f64,
    current_lmp: f64,
) -> f64 {
    net_mw * (current_lmp - avg_fill_price)
}

#[no_mangle]
pub extern "C" fn calc_net_exposure(net_mw: f64, _current_lmp: f64) -> f64 {
    net_mw.abs()
}

#[no_mangle]
pub extern "C" fn check_limit_breach(
    net_exposure_mw: f64,
    position_limit_mw: f64,
) -> c_int {
    if net_exposure_mw > position_limit_mw { 1 } else { 0 }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mtm_pnl_long_profit() {
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 45.0), 50.0);
    }

    #[test]
    fn mtm_pnl_long_loss() {
        assert_eq!(calc_mtm_pnl(10.0, 40.0, 35.0), -50.0);
    }

    #[test]
    fn mtm_pnl_short() {
        // short -5 MW, avg 50, current 45: -5 * (45-50) = +25
        assert_eq!(calc_mtm_pnl(-5.0, 50.0, 45.0), 25.0);
    }

    #[test]
    fn net_exposure_positive() {
        assert_eq!(calc_net_exposure(10.0, 45.0), 10.0);
    }

    #[test]
    fn net_exposure_negative() {
        assert_eq!(calc_net_exposure(-7.0, 45.0), 7.0);
    }

    #[test]
    fn net_exposure_ignores_lmp() {
        assert_eq!(
            calc_net_exposure(5.0, 0.0),
            calc_net_exposure(5.0, 999.0),
        );
    }

    #[test]
    fn limit_breach_over() {
        assert_eq!(check_limit_breach(51.0, 50.0), 1);
    }

    #[test]
    fn limit_breach_under() {
        assert_eq!(check_limit_breach(49.0, 50.0), 0);
    }

    #[test]
    fn limit_breach_exact() {
        // equal is not a breach — strict >
        assert_eq!(check_limit_breach(50.0, 50.0), 0);
    }
}
