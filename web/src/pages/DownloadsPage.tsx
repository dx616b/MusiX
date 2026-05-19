import { useCallback, useEffect, useState } from 'react'
import { listDownloads, type Download } from '../api'

export default function DownloadsPage() {
  const [rows, setRows] = useState<Download[]>([])
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const data = await listDownloads()
      setRows(data.downloads)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load downloads')
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  return (
    <div className="page">
      <h1>Downloads</h1>
      <p className="muted">
        Files land in Transmission&apos;s download folder. Point Navidrome at that path to build your library.
      </p>
      {error && <p className="error">{error}</p>}
      <ul className="list">
        {rows.map((d) => (
          <li key={d.id} className="row download-row">
            <div className="result-meta">
              <strong>{d.title}</strong>
              <span className="muted">
                {d.status} · {d.percentDone.toFixed(1)}% · {d.indexer || 'unknown indexer'}
              </span>
            </div>
          </li>
        ))}
      </ul>
      {rows.length === 0 && !error && <p className="muted">No downloads yet.</p>}
    </div>
  )
}
