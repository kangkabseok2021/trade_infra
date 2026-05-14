mod backtest;
mod execution;
mod indicators;
mod metrics;
mod portfolio;
pub mod types;

use pyo3::prelude::*;
use types::{Bar, EquityPoint, Trade};

#[pyclass]
#[derive(Clone)]
pub struct MetricsPy {
    #[pyo3(get)] pub total_return: f64,
    #[pyo3(get)] pub annualized_return: f64,
    #[pyo3(get)] pub sharpe: f64,
    #[pyo3(get)] pub sortino: f64,
    #[pyo3(get)] pub max_drawdown: f64,
    #[pyo3(get)] pub max_drawdown_duration: usize,
    #[pyo3(get)] pub win_rate: f64,
    #[pyo3(get)] pub avg_win: f64,
    #[pyo3(get)] pub avg_loss: f64,
    #[pyo3(get)] pub num_trades: usize,
}

#[pyclass]
#[derive(Clone)]
pub struct BacktestResult {
    #[pyo3(get)] pub equity_curve: Vec<EquityPoint>,
    #[pyo3(get)] pub trades: Vec<Trade>,
    #[pyo3(get)] pub metrics: MetricsPy,
}

#[pyclass]
pub struct BacktestEngine {
    capital: f64,
    slippage_bps: f64,
    commission: f64,
}

#[pymethods]
impl BacktestEngine {
    #[new]
    fn new(capital: f64, slippage_bps: f64, commission: f64) -> Self {
        BacktestEngine { capital, slippage_bps, commission }
    }

    fn run(&self, bars: Vec<Bar>, signals: Vec<f64>, symbol: &str) -> PyResult<BacktestResult> {
        if bars.len() != signals.len() {
            return Err(pyo3::exceptions::PyValueError::new_err(
                format!("bars ({}) and signals ({}) must have equal length", bars.len(), signals.len()),
            ));
        }
        let config = backtest::BacktestConfig {
            capital: self.capital,
            slippage_bps: self.slippage_bps,
            commission: self.commission,
            symbol: symbol.to_string(),
        };
        let result = backtest::run(&bars, &signals, &config);
        let m = metrics::compute(&result.equity_curve, &result.trades);
        Ok(BacktestResult {
            equity_curve: result.equity_curve,
            trades: result.trades,
            metrics: MetricsPy {
                total_return: m.total_return,
                annualized_return: m.annualized_return,
                sharpe: m.sharpe,
                sortino: m.sortino,
                max_drawdown: m.max_drawdown,
                max_drawdown_duration: m.max_drawdown_duration,
                win_rate: m.win_rate,
                avg_win: m.avg_win,
                avg_loss: m.avg_loss,
                num_trades: m.num_trades,
            },
        })
    }
}

#[pyfunction]
fn sma(prices: Vec<f64>, period: usize) -> Vec<f64> {
    indicators::sma(&prices, period)
}

#[pyfunction]
fn ema(prices: Vec<f64>, period: usize) -> Vec<f64> {
    indicators::ema(&prices, period)
}

#[pyfunction]
fn rsi(prices: Vec<f64>, period: usize) -> Vec<f64> {
    indicators::rsi(&prices, period)
}

#[pymodule]
fn quant_engine(m: &Bound<'_, PyModule>) -> PyResult<()> {
    m.add_class::<Bar>()?;
    m.add_class::<EquityPoint>()?;
    m.add_class::<Trade>()?;
    m.add_class::<MetricsPy>()?;
    m.add_class::<BacktestEngine>()?;
    m.add_class::<BacktestResult>()?;
    m.add_function(wrap_pyfunction!(sma, m)?)?;
    m.add_function(wrap_pyfunction!(ema, m)?)?;
    m.add_function(wrap_pyfunction!(rsi, m)?)?;
    Ok(())
}
