use crate::execution::ExecutionEngine;
use crate::portfolio::Portfolio;
use crate::types::{Bar, EquityPoint, Order, Side, Trade};

pub struct BacktestConfig {
    pub capital: f64,
    pub slippage_bps: f64,
    pub commission: f64,
    pub symbol: String,
}

pub struct BacktestResult {
    pub equity_curve: Vec<EquityPoint>,
    pub trades: Vec<Trade>,
}

pub fn run(bars: &[Bar], signals: &[f64], config: &BacktestConfig) -> BacktestResult {
    assert_eq!(bars.len(), signals.len(), "bars and signals must have equal length");

    let engine = ExecutionEngine::new(config.slippage_bps, config.commission);
    let mut portfolio = Portfolio::new(config.capital);
    let mut pending_order: Option<Order> = None;
    let mut prev_signal = 0.0f64;

    for (i, bar) in bars.iter().enumerate() {
        // Fill any order queued from the previous bar (fill at this bar's open)
        if let Some(ref order) = pending_order {
            let fill_price = engine.fill_price(bar.open, &order.side);
            portfolio.fill(order, fill_price, engine.commission());
        }
        pending_order = None;

        // Record equity using today's close
        portfolio.record_equity(bar.date, bar.close);

        // Detect signal transitions, queue order for next bar's open
        let sig = signals[i];
        if sig > 0.0 && prev_signal <= 0.0 && portfolio.cash() > 0.0 {
            let qty = (portfolio.cash() * 0.99) / bar.close;
            if qty > 0.0 {
                pending_order = Some(Order {
                    date: bar.date,
                    symbol: config.symbol.clone(),
                    qty,
                    side: Side::Buy,
                });
            }
        } else if sig <= 0.0 && prev_signal > 0.0 {
            pending_order = Some(Order {
                date: bar.date,
                symbol: config.symbol.clone(),
                qty: 0.0, // portfolio.fill sells entire position when Side::Sell
                side: Side::Sell,
            });
        }
        prev_signal = sig;
    }

    BacktestResult {
        equity_curve: portfolio.equity_curve().to_vec(),
        trades: portfolio.trades().to_vec(),
    }
}
