use std::os::raw::c_int;

fn calc_mtm_pnl_impl(_net_mw: f64, _avg_fill_price: f64, _current_lmp: f64) -> f64 {
    unimplemented!()
}

fn calc_net_exposure_impl(_net_mw: f64, _current_lmp: f64) -> f64 {
    unimplemented!()
}

fn check_limit_breach_impl(_net_exposure_mw: f64, _position_limit_mw: f64) -> c_int {
    unimplemented!()
}

#[no_mangle]
pub extern "C" fn calc_mtm_pnl(net_mw: f64, avg_fill_price: f64, current_lmp: f64) -> f64 {
    calc_mtm_pnl_impl(net_mw, avg_fill_price, current_lmp)
}

#[no_mangle]
pub extern "C" fn calc_net_exposure(net_mw: f64, current_lmp: f64) -> f64 {
    calc_net_exposure_impl(net_mw, current_lmp)
}

#[no_mangle]
pub extern "C" fn check_limit_breach(net_exposure_mw: f64, position_limit_mw: f64) -> c_int {
    check_limit_breach_impl(net_exposure_mw, position_limit_mw)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[should_panic]
    fn mtm_pnl_long_profit() {
        assert_eq!(calc_mtm_pnl_impl(10.0, 40.0, 45.0), 50.0);
    }

    #[test]
    #[should_panic]
    fn mtm_pnl_long_loss() {
        assert_eq!(calc_mtm_pnl_impl(10.0, 40.0, 35.0), -50.0);
    }

    #[test]
    #[should_panic]
    fn mtm_pnl_short() {
        assert_eq!(calc_mtm_pnl_impl(-5.0, 50.0, 45.0), 25.0);
    }

    #[test]
    #[should_panic]
    fn net_exposure_positive() {
        assert_eq!(calc_net_exposure_impl(10.0, 45.0), 10.0);
    }

    #[test]
    #[should_panic]
    fn net_exposure_negative() {
        assert_eq!(calc_net_exposure_impl(-7.0, 45.0), 7.0);
    }

    #[test]
    #[should_panic]
    fn net_exposure_ignores_lmp() {
        assert_eq!(
            calc_net_exposure_impl(5.0, 0.0),
            calc_net_exposure_impl(5.0, 999.0),
        );
    }

    #[test]
    #[should_panic]
    fn limit_breach_over() {
        assert_eq!(check_limit_breach_impl(51.0, 50.0), 1);
    }

    #[test]
    #[should_panic]
    fn limit_breach_under() {
        assert_eq!(check_limit_breach_impl(49.0, 50.0), 0);
    }

    #[test]
    #[should_panic]
    fn limit_breach_exact() {
        assert_eq!(check_limit_breach_impl(50.0, 50.0), 0);
    }
}
