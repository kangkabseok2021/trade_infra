pub fn sma(prices: &[f64], period: usize) -> Vec<f64> {
    let n = prices.len();
    let mut out = vec![f64::NAN; n];
    if period == 0 || period > n {
        return out;
    }
    let mut sum: f64 = prices[..period].iter().sum();
    out[period - 1] = sum / period as f64;
    for i in period..n {
        sum += prices[i] - prices[i - period];
        out[i] = sum / period as f64;
    }
    out
}

pub fn ema(prices: &[f64], period: usize) -> Vec<f64> {
    let n = prices.len();
    let mut out = vec![f64::NAN; n];
    if period == 0 || period > n {
        return out;
    }
    let k = 2.0 / (period as f64 + 1.0);
    let seed: f64 = prices[..period].iter().sum::<f64>() / period as f64;
    out[period - 1] = seed;
    for i in period..n {
        out[i] = prices[i] * k + out[i - 1] * (1.0 - k);
    }
    out
}

pub fn rsi(prices: &[f64], period: usize) -> Vec<f64> {
    let n = prices.len();
    let mut out = vec![f64::NAN; n];
    if period == 0 || n <= period {
        return out;
    }
    let changes: Vec<f64> = prices.windows(2).map(|w| w[1] - w[0]).collect();
    let (mut avg_gain, mut avg_loss) = changes[..period]
        .iter()
        .fold((0.0f64, 0.0f64), |(g, l), &c| {
            if c > 0.0 { (g + c, l) } else { (g, l + c.abs()) }
        });
    avg_gain /= period as f64;
    avg_loss /= period as f64;

    let rs = if avg_loss == 0.0 { f64::INFINITY } else { avg_gain / avg_loss };
    out[period] = 100.0 - 100.0 / (1.0 + rs);

    for i in (period + 1)..n {
        let c = changes[i - 1];
        let (gain, loss) = if c > 0.0 { (c, 0.0) } else { (0.0, c.abs()) };
        avg_gain = (avg_gain * (period as f64 - 1.0) + gain) / period as f64;
        avg_loss = (avg_loss * (period as f64 - 1.0) + loss) / period as f64;
        let rs = if avg_loss == 0.0 { f64::INFINITY } else { avg_gain / avg_loss };
        out[i] = 100.0 - 100.0 / (1.0 + rs);
    }
    out
}
