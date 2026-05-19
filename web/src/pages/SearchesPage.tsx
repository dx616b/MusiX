import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import TorrentPreviewPanel from '../components/TorrentPreviewPanel'
import {
  formatBytes,
  getStoredSearch,
  listSearches,
  magnetForResult,
  queueDownload,
  type SearchHistory,
  type SearchResult,
} from '../api'

function formatWhen(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

export default function SearchesPage() {
  const [rows, setRows] = useState<SearchHistory[]>([])
  const [selected, setSelected] = useState<SearchHistory | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<string | null>(null)
  const [queueing, setQueueing] = useState<string | null>(null)
  const [preview, setPreview] = useState<SearchResult | null>(null)

  const refresh = useCallback(async () => {
    try {
      const data = await listSearches(100)
      setRows(data.searches ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load searches')
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  async function openCached(query: string) {
    setLoading(query)
    setError(null)
    try {
      const row = await getStoredSearch(query)
      setSelected(row)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load saved results')
    } finally {
      setLoading(null)
    }
  }

  async function onDownload(r: SearchResult) {
    if (!selected) return
    const magnet = magnetForResult(r)
    if (!magnet) {
      setError('No magnet, info hash, or download URL for this release')
      return
    }
    setQueueing(r.title)
    setError(null)
    try {
      await queueDownload({
        query: selected.query,
        title: r.title,
        magnet,
        infoHash: r.infoHash,
        indexer: r.indexer,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Download failed')
    } finally {
      setQueueing(null)
    }
  }

  return (
    <div className="page">
      <h1>Search history</h1>
      <p className="muted">Saved queries and results. Download directly or run a fresh search.</p>
      {error && <p className="error">{error}</p>}
      <ul className="list">
        {rows.map((s) => (
          <li key={`${s.query}-${s.searchedAt}`} className="row history-row">
            <div className="result-meta">
              <button type="button" className="text-btn query-link" onClick={() => openCached(s.query)}>
                {s.query}
              </button>
              <span className="muted">
                {s.resultCount} results · {formatWhen(s.searchedAt)}
              </span>
            </div>
            <div className="history-actions">
              <button
                type="button"
                className="btn-link"
                disabled={loading === s.query}
                onClick={() => openCached(s.query)}
              >
                {loading === s.query ? 'Loading…' : 'Show'}
              </button>
              <Link className="btn-link" to={`/?q=${encodeURIComponent(s.query)}`}>
                Search fresh
              </Link>
            </div>
          </li>
        ))}
      </ul>
      {rows.length === 0 && !error && <p className="muted">No searches yet.</p>}

      {selected && (
        <section className="card cached-results">
          <div className="cached-results-header">
            <h2>{selected.query}</h2>
            <span className="muted">
              {selected.resultCount} saved · {formatWhen(selected.searchedAt)}
            </span>
            <Link className="btn-link" to={`/?q=${encodeURIComponent(selected.query)}`}>
              Search fresh
            </Link>
          </div>
          <ul className="list">
            {(selected.results ?? []).map((r) => (
              <li key={`${r.infoHash || r.title}-${r.indexer}`} className="row result-row">
                <div className="result-meta">
                  <strong>{r.title}</strong>
                  <span className="muted">
                    {r.indexer} · {formatBytes(r.size)} · {r.seeders} seeders
                  </span>
                </div>
                <div className="row-actions">
                  <button type="button" className="btn-secondary" onClick={() => setPreview(r)}>
                    Preview
                  </button>
                  <button
                    type="button"
                    disabled={queueing === r.title}
                    onClick={() => onDownload(r)}
                  >
                    {queueing === r.title ? 'Adding…' : 'Download'}
                  </button>
                </div>
              </li>
            ))}
          </ul>
          {(selected.results?.length ?? 0) === 0 && (
            <p className="muted">No results were stored for this query. Use Search fresh.</p>
          )}
        </section>
      )}
      {preview && (
        <TorrentPreviewPanel
          result={preview}
          query={selected?.query}
          onClose={() => setPreview(null)}
        />
      )}
    </div>
  )
}
