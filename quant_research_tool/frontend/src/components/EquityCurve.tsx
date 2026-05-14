import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { EquityPoint } from '../types'

interface Props { data: EquityPoint[] }

function computeDrawdownPct(data: EquityPoint[]): number[] {
  let peak = data[0]?.value ?? 0
  return data.map(ep => {
    if (ep.value > peak) peak = ep.value
    return peak > 0 ? +((( ep.value - peak) / peak) * 100).toFixed(2) : 0
  })
}

const fmtDate = (ts: number): string =>
  new Date(ts * 1000).toLocaleDateString('en-US', { month: 'short', year: '2-digit' })

export default function EquityCurve({ data }: Props) {
  const drawdowns = computeDrawdownPct(data)
  const chartData = data.map((ep, i) => ({
    date: fmtDate(ep.date),
    equity: +ep.value.toFixed(2),
    drawdown: drawdowns[i],
  }))

  return (
    <div className="chart-container">
      <h3>Equity Curve &amp; Drawdown</h3>
      <ResponsiveContainer width="100%" height={340}>
        <AreaChart data={chartData} margin={{ top: 10, right: 40, left: 10, bottom: 0 }}>
          <defs>
            <linearGradient id="colorEquity" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#22c55e" stopOpacity={0.25} />
              <stop offset="95%" stopColor="#22c55e" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="colorDD" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#f87171" stopOpacity={0.25} />
              <stop offset="95%" stopColor="#f87171" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="#1e2a3a" />
          <XAxis dataKey="date" tick={{ fontSize: 11, fill: '#64748b' }} tickLine={false} />
          <YAxis
            yAxisId="equity"
            orientation="left"
            tick={{ fontSize: 11, fill: '#64748b' }}
            tickFormatter={v => `$${(v / 1000).toFixed(0)}k`}
            tickLine={false}
          />
          <YAxis
            yAxisId="drawdown"
            orientation="right"
            tick={{ fontSize: 11, fill: '#64748b' }}
            tickFormatter={v => `${v}%`}
            tickLine={false}
          />
          <Tooltip
            contentStyle={{ background: '#1e2336', border: '1px solid #2a3050', borderRadius: 6 }}
            labelStyle={{ color: '#94a3b8', fontSize: 11 }}
          />
          <Legend wrapperStyle={{ fontSize: 12, paddingTop: 8 }} />
          <Area
            yAxisId="equity"
            type="monotone"
            dataKey="equity"
            name="Portfolio Value ($)"
            stroke="#22c55e"
            strokeWidth={2}
            fill="url(#colorEquity)"
          />
          <Area
            yAxisId="drawdown"
            type="monotone"
            dataKey="drawdown"
            name="Drawdown (%)"
            stroke="#f87171"
            strokeWidth={1.5}
            fill="url(#colorDD)"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
