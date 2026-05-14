use crate::types::Side;

pub struct ExecutionEngine {
    slippage_bps: f64,
    commission: f64,
}

impl ExecutionEngine {
    pub fn new(slippage_bps: f64, commission: f64) -> Self {
        ExecutionEngine { slippage_bps, commission }
    }

    pub fn fill_price(&self, open: f64, side: &Side) -> f64 {
        let slip = open * self.slippage_bps / 10_000.0;
        match side {
            Side::Buy => open + slip,
            Side::Sell => open - slip,
        }
    }

    pub fn commission(&self) -> f64 { self.commission }
}
