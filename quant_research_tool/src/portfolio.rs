use crate::types::{EquityPoint, Order, Side, Trade};

pub struct Portfolio {
    cash: f64,
    position_qty: f64,
    position_cost: f64,
    position_entry_date: i64,
    position_entry_commission: f64,
    trades: Vec<Trade>,
    equity_curve: Vec<EquityPoint>,
}

impl Portfolio {
    pub fn new(capital: f64) -> Self {
        Portfolio {
            cash: capital,
            position_qty: 0.0,
            position_cost: 0.0,
            position_entry_date: 0,
            position_entry_commission: 0.0,
            trades: Vec::new(),
            equity_curve: Vec::new(),
        }
    }

    pub fn fill(&mut self, order: &Order, fill_price: f64, commission: f64) {
        match order.side {
            Side::Buy => {
                if self.position_qty == 0.0 {
                    let affordable_qty = ((self.cash - commission).max(0.0) / fill_price).min(order.qty);
                    if affordable_qty > 0.0 {
                        let cost = affordable_qty * fill_price + commission;
                        self.cash -= cost;
                        self.position_qty = affordable_qty;
                        self.position_cost = fill_price;
                        self.position_entry_date = order.date;
                        self.position_entry_commission = commission;
                    }
                }
            }
            Side::Sell => {
                if self.position_qty > 0.0 {
                    let proceeds = self.position_qty * fill_price - commission;
                    let entry_cost = self.position_qty * self.position_cost + self.position_entry_commission;
                    let gross_entry = self.position_qty * self.position_cost;
                    let pnl = proceeds - entry_cost;
                    let pnl_pct = if gross_entry != 0.0 { pnl / gross_entry } else { 0.0 };
                    self.cash += proceeds;
                    self.trades.push(Trade {
                        entry_date: self.position_entry_date,
                        exit_date: order.date,
                        symbol: order.symbol.clone(),
                        pnl,
                        pnl_pct,
                    });
                    self.position_qty = 0.0;
                    self.position_cost = 0.0;
                    self.position_entry_date = 0;
                    self.position_entry_commission = 0.0;
                }
            }
        }
    }

    pub fn record_equity(&mut self, date: i64, current_price: f64) {
        let market_value = self.position_qty * current_price;
        self.equity_curve.push(EquityPoint { date, value: self.cash + market_value });
    }

    pub fn cash(&self) -> f64 { self.cash }
    pub fn trades(&self) -> &[Trade] { &self.trades }
    pub fn equity_curve(&self) -> &[EquityPoint] { &self.equity_curve }
}
