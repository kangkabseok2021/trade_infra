use crate::types::{EquityPoint, Trade};

pub struct Metrics {
    pub total_return: f64,
    pub annualized_return: f64,
    pub sharpe: f64,
    pub sortino: f64,
    pub max_drawdown: f64,
    pub max_drawdown_duration: usize,
    pub win_rate: f64,
    pub avg_win: f64,
    pub avg_loss: f64,
    pub num_trades: usize,
}

pub fn compute(equity_curve: &[EquityPoint], trades: &[Trade]) -> Metrics {
    let n = equity_curve.len();
    let zero = Metrics {
        total_return: 0.0, annualized_return: 0.0, sharpe: 0.0, sortino: 0.0,
        max_drawdown: 0.0, max_drawdown_duration: 0, win_rate: 0.0,
        avg_win: 0.0, avg_loss: 0.0, num_trades: 0,
    };
    if n < 2 { return zero; }

    let initial = equity_curve[0].value;
    let final_val = equity_curve[n - 1].value;
    let total_return = (final_val - initial) / initial;

    let returns: Vec<f64> = equity_curve.windows(2)
        .map(|w| (w[1].value - w[0].value) / w[0].value)
        .collect();

    let nr = returns.len() as f64;
    let mean_r = returns.iter().sum::<f64>() / nr;
    let variance = if nr > 1.0 {
        returns.iter().map(|r| (r - mean_r).powi(2)).sum::<f64>() / (nr - 1.0)
    } else {
        0.0
    };
    let std_dev = variance.sqrt();

    let annualized_return = (1.0 + total_return).powf(252.0 / nr) - 1.0;
    let sharpe = if std_dev == 0.0 { 0.0 } else { mean_r / std_dev * 252.0_f64.sqrt() };

    let downside_returns: Vec<f64> = returns.iter().filter(|&&r| r < 0.0).cloned().collect();
    let n_down = downside_returns.len() as f64;
    let downside_dev = if n_down == 0.0 {
        0.0
    } else {
        (downside_returns.iter().map(|&r| r.powi(2)).sum::<f64>() / n_down).sqrt()
    };
    let sortino = if downside_dev == 0.0 { 0.0 } else { mean_r / downside_dev * 252.0_f64.sqrt() };

    let mut peak = equity_curve[0].value;
    let mut max_dd = 0.0f64;
    let mut cur_peak_idx = 0usize;
    let mut max_dd_dur = 0usize;

    for (i, ep) in equity_curve.iter().enumerate() {
        if ep.value > peak {
            peak = ep.value;
            cur_peak_idx = i;
        }
        let dd = (peak - ep.value) / peak;
        if dd > max_dd {
            max_dd = dd;
            max_dd_dur = i - cur_peak_idx;
        }
    }

    let num_trades = trades.len();
    let wins: Vec<f64> = trades.iter().filter(|t| t.pnl > 0.0).map(|t| t.pnl).collect();
    let losses: Vec<f64> = trades.iter().filter(|t| t.pnl <= 0.0).map(|t| t.pnl.abs()).collect();
    let win_rate = if num_trades == 0 { 0.0 } else { wins.len() as f64 / num_trades as f64 };
    let avg_win = if wins.is_empty() { 0.0 } else { wins.iter().sum::<f64>() / wins.len() as f64 };
    let avg_loss = if losses.is_empty() { 0.0 } else { losses.iter().sum::<f64>() / losses.len() as f64 };

    Metrics { total_return, annualized_return, sharpe, sortino, max_drawdown: max_dd,
               max_drawdown_duration: max_dd_dur, win_rate, avg_win, avg_loss, num_trades }
}
