import { useState } from 'react'
import EquityCurve from './components/EquityCurve'
import MetricsCards from './components/MetricsCards'
import RunForm from './components/RunForm'
import type { BacktestResult } from './types'
import './index.css'

export default function App() {
  const [result, setResult] = useState<BacktestResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleRun = async (strategyName: string) => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch('/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ strategy_name: strategyName }),
      })
      if (!res.ok) {
        const msg = await res.text()
        throw new Error(`${res.status}: ${msg}`)
      }
      setResult(await res.json())
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="app">
      <header>
        <h1>Backtesting Engine</h1>
        <p className="subtitle">Rust-powered · PyO3 bindings · Hybrid execution</p>
      </header>
      <RunForm onRun={handleRun} loading={loading} />
      {error && <p className="error">{error}</p>}
      {result && (
        <>
          <MetricsCards metrics={result.metrics} />
          <EquityCurve data={result.equity_curve} />
        </>
      )}
    </div>
  )
}
