export type EquityPoint = {
  date: number;   // Unix timestamp (seconds)
  value: number;
};

export type Metrics = {
  total_return: number;
  annualized_return: number;
  sharpe: number;
  sortino: number;
  max_drawdown: number;
  win_rate: number;
  num_trades: number;
};

export type Trade = {
  entry_date: number;
  exit_date: number;
  symbol: string;
  pnl: number;
  pnl_pct: number;
};

export type BacktestResult = {
  equity_curve: EquityPoint[];
  metrics: Metrics;
  trades: Trade[];
};
