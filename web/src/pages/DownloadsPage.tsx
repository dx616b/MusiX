import { useCallback, useEffect, useMemo, useState } from 'react'
import SearchQueryLink from '../components/SearchQueryLink'
import { listDownloads, listSearches, normalizeSearchQuery, type Download } from '../api'

function formatWhen(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

function statusBucket(status: string, percentDone: number): 'active' | 'done' | 'other' {
  const s = status.toLowerCase()
  if (percentDone >= 100 || s.includes('seed')) return 'done'
  if (s.includes('error')) return 'other'
  if (percentDone > 0 && percentDone < 100) return 'active'
  if (s.includes('download') || s.includes('queue') || s.includes('check')) return 'active'
  return 'other'
}

export default function DownloadsPage() {
  const [rows, setRows] = useState<Download[]>([])
  const [historyKeys, setHistoryKeys] = useState<Set<string>>(() => new Set())
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [filterText, setFilterText] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'done'>('all')

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const [downloads, searches] = await Promise.all([listDownloads(), listSearches(200)])
      setRows(downloads.downloads ?? [])
      setHistoryKeys(
        new Set((searches.searches ?? []).map((s) => normalizeSearchQuery(s.query))),
      )
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load downloads')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  const filtered = useMemo(() => {
    const text = filterText.trim().toLowerCase()
    return rows.filter((d) => {
      if (statusFilter !== 'all') {
        const bucket = statusBucket(d.status, d.percentDone)
        if (statusFilter === 'active' && bucket !== 'active') return false
        if (statusFilter === 'done' && bucket !== 'done') return false
      }
      if (!text) return true
      const hay = `${d.title} ${d.query ?? ''} ${d.indexer ?? ''} ${d.status}`.toLowerCase()
      return hay.includes(text)
    })
  }, [rows, filterText, statusFilter])

  return (
    <div className="page">
      <h1>Downloads</h1>
      <p className="muted">
        Queued in Transmission. List refreshes every 5 seconds.
      </p>

      <div className="downloads-toolbar">
        <label className="downloads-search">
          <span className="muted">Search</span>
          <input
            type="search"
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
            placeholder="Title, query, indexer, status…"
          />
        </label>
        <label className="downloads-status-filter">
          <span className="muted">Show</span>
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}>
            <option value="all">All</option>
            <option value="active">In progress</option>
            <option value="done">Finished</option>
          </select>
        </label>
        <button type="button" className="btn-secondary" disabled={loading} onClick={() => void refresh()}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </div>

      {rows.length > 0 && (
        <p className="muted filters-summary">
          Showing {filtered.length} of {rows.length} downloads
        </p>
      )}

      {error && <p className="error">{error}</p>}
      <ul className="list">
        {filtered.map((d) => (
          <li key={d.id} className="row download-row">
            <div className="result-meta">
              <strong>{d.title}</strong>
              {d.query && (
                <span className="muted">
                  Search: <SearchQueryLink query={d.query} historyKeys={historyKeys} />
                </span>
              )}
              <span className="muted">
                {d.status} · {d.indexer || 'unknown indexer'} · updated {formatWhen(d.updatedAt)}
              </span>
              <div
                className="progress-track"
                role="progressbar"
                aria-valuenow={Math.round(d.percentDone)}
                aria-valuemin={0}
                aria-valuemax={100}
              >
                <div
                  className="progress-fill"
                  style={{ width: `${Math.min(100, Math.max(0, d.percentDone))}%` }}
                />
              </div>
              <span className="muted">{d.percentDone.toFixed(1)}%</span>
            </div>
          </li>
        ))}
      </ul>
      {rows.length === 0 && !error && !loading && <p className="muted">No downloads yet. Queue one from Search.</p>}
      {rows.length > 0 && filtered.length === 0 && !error && (
        <p className="muted">No downloads match your filters.</p>
      )}
    </div>
  )
}
