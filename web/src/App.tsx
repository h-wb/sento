import { useEffect, useState } from 'react'
import './App.css'

type SystemStatus = {
  app: string
  version: string
  commit: string
  database: { ok: boolean }
}

function App() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/v1/system/status')
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then(setStatus)
      .catch((e) => setError(String(e)))
  }, [])

  return (
    <main className="shell">
      <h1>sento</h1>
      <p className="tagline">manga automation, the *arr way</p>
      {error && <p className="error">backend unreachable: {error}</p>}
      {status && (
        <dl className="status">
          <dt>version</dt>
          <dd>{status.version}</dd>
          <dt>database</dt>
          <dd>{status.database.ok ? 'ok' : 'down'}</dd>
        </dl>
      )}
    </main>
  )
}

export default App
