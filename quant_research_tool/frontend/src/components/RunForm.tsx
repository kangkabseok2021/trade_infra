import { useEffect, useState } from 'react'

interface Props {
  onRun: (strategyName: string) => void
  loading: boolean
}

export default function RunForm({ onRun, loading }: Props) {
  const [strategies, setStrategies] = useState<string[]>([])
  const [selected, setSelected] = useState('')

  useEffect(() => {
    fetch('/strategies')
      .then(r => r.json())
      .then((names: string[]) => {
        setStrategies(names)
        if (names.length > 0) setSelected(names[0])
      })
      .catch(console.error)
  }, [])

  return (
    <div className="run-form">
      <label htmlFor="strategy-select">Strategy</label>
      <select
        id="strategy-select"
        value={selected}
        onChange={e => setSelected(e.target.value)}
        disabled={loading}
      >
        {strategies.length === 0 && <option value="">No strategies loaded</option>}
        {strategies.map(s => (
          <option key={s} value={s}>{s}</option>
        ))}
      </select>
      <button
        onClick={() => selected && onRun(selected)}
        disabled={loading || !selected}
      >
        {loading ? 'Running…' : 'Run ▶'}
      </button>
    </div>
  )
}
