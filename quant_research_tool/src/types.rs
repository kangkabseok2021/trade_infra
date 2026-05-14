use pyo3::prelude::*;

#[derive(Clone, Debug)]
#[pyclass]
pub struct Bar {
    #[pyo3(get, set)]
    pub date: i64,
    #[pyo3(get, set)]
    pub open: f64,
    #[pyo3(get, set)]
    pub high: f64,
    #[pyo3(get, set)]
    pub low: f64,
    #[pyo3(get, set)]
    pub close: f64,
    #[pyo3(get, set)]
    pub volume: f64,
}

#[pymethods]
impl Bar {
    #[new]
    pub fn new(date: i64, open: f64, high: f64, low: f64, close: f64, volume: f64) -> Self {
        Bar { date, open, high, low, close, volume }
    }
}

#[derive(Clone, Debug, PartialEq)]
pub enum Side { Buy, Sell }

#[derive(Clone, Debug)]
pub struct Order {
    pub date: i64,
    pub symbol: String,
    pub qty: f64,
    pub side: Side,
}

#[derive(Clone, Debug)]
#[pyclass]
pub struct Trade {
    #[pyo3(get)]
    pub entry_date: i64,
    #[pyo3(get)]
    pub exit_date: i64,
    #[pyo3(get)]
    pub symbol: String,
    #[pyo3(get)]
    pub pnl: f64,
    #[pyo3(get)]
    pub pnl_pct: f64,
}

#[derive(Clone, Debug)]
#[pyclass]
pub struct EquityPoint {
    #[pyo3(get)]
    pub date: i64,
    #[pyo3(get)]
    pub value: f64,
}
