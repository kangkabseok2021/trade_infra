import type { Metrics } from '../types'

interface Props { metrics: Metrics }

const pct = (n: number): string => `${(n * 100).toFixed(2)}%`
const dec = (n: number): string => n.toFixed(2)

export default function MetricsCards({ metrics }: Props) {
  const cards = [
    {
      label: 'Total Return',
      value: pct(metrics.total_return),
      positive: metrics.total_return >= 0,
    },
    {
      label: 'Sharpe Ratio',
      value: dec(metrics.sharpe),
      positive: metrics.sharpe >= 1,
    },
    {
      label: 'Sortino Ratio',
      value: dec(metrics.sortino),
      positive: metrics.sortino >= 1,
    },
    {
      label: 'Max Drawdown',
      value: pct(metrics.max_drawdown),
      positive: metrics.max_drawdown < 0.1,
    },
    {
      label: 'Win Rate',
      value: pct(metrics.win_rate),
      positive: metrics.win_rate >= 0.5,
    },
    {
      label: '# Trades',
      value: String(metrics.num_trades),
      positive: true,
    },
  ]

  return (
    <div className="metrics-cards">
      {cards.map(c => (
        <div key={c.label} className={`metric-card ${c.positive ? 'positive' : 'negative'}`}>
          <span className="metric-label">{c.label}</span>
          <span className="metric-value">{c.value}</span>
        </div>
      ))}
    </div>
  )
}
