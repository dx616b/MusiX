import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import TorrentPreviewPanel from '../components/TorrentPreviewPanel'
import {
  clearAllStoredSearches,
  deleteStoredSearch,
  formatBytes,
  getStoredSearch,
  listSearches,
  magnetForResult,
  normalizeSearchQuery,
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
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const cachedResultsRef = useRef<HTMLElement>(null)
  const [rows, setRows] = useState<SearchHistory[]>([])
  const [selected, setSelected] = useState<SearchHistory | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<string | null>(null)
  const [queueing, setQueueing] = useState<string | null>(null)
  const [preview, setPreview] = useState<SearchResult | null>(null)
  const [removing, setRemoving] = useState<string | null>(null)
  const [clearing, setClearing] = useState(false)
  const [historyLoaded, setHistoryLoaded] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const data = await listSearches(100)
      setRows(data.searches ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load searches')
    } finally {
      setHistoryLoaded(true)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const scrollToCachedResults = useCallback(() => {
    requestAnimationFrame(() => {
      cachedResultsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }, [])

  const openCached = useCallback(
    async (query: string, options?: { redirectIfMissing?: boolean; scroll?: boolean }) => {
      const trimmed = query.trim()
      if (!trimmed) return
      setLoading(trimmed)
      setError(null)
      try {
        const row = await getStoredSearch(trimmed)
        setSelected(row)
        if (options?.scroll) scrollToCachedResults()
      } catch (err) {
        const match = rows.find(
          (r) => normalizeSearchQuery(r.query) === normalizeSearchQuery(trimmed),
        )
        if (match && match.query !== trimmed) {
          try {
            const row = await getStoredSearch(match.query)
            setSelected(row)
            navigate(`/searches?q=${encodeURIComponent(match.query)}`, { replace: true })
            if (options?.scroll) scrollToCachedResults()
            return
          } catch {
            /* try redirect below */
          }
        }
        if (options?.redirectIfMissing) {
          navigate(`/?q=${encodeURIComponent(trimmed)}`)
          return
        }
        setError(err instanceof Error ? err.message : 'Failed to load saved results')
      } finally {
        setLoading(null)
      }
    },
    [navigate, rows, scrollToCachedResults],
  )

  const qFromUrl = searchParams.get('q')?.trim() ?? ''

  useEffect(() => {
    if (!qFromUrl || !historyLoaded) return
    void openCached(qFromUrl, { redirectIfMissing: true, scroll: true })
  }, [qFromUrl, historyLoaded, openCached])

  async function removeOne(query: string) {
    setRemoving(query)
    setError(null)
    try {
      await deleteStoredSearch(query)
      if (selected?.query === query) setSelected(null)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove search')
    } finally {
      setRemoving(null)
    }
  }

  async function clearAll() {
    if (!window.confirm('Remove all saved searches and cached results?')) return
    setClearing(true)
    setError(null)
    try {
      await clearAllStoredSearches()
      setSelected(null)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to clear history')
    } finally {
      setClearing(false)
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
      {rows.length > 0 && (
        <p>
          <button
            type="button"
            className="btn-link danger"
            disabled={clearing}
            onClick={() => void clearAll()}
          >
            {clearing ? 'Clearing…' : 'Clear all'}
          </button>
        </p>
      )}
      {qFromUrl && loading === qFromUrl && (
        <p className="muted">Opening saved results for &ldquo;{qFromUrl}&rdquo;…</p>
      )}
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
              <button
                type="button"
                className="text-btn danger"
                disabled={removing === s.query}
                onClick={() => void removeOne(s.query)}
              >
                {removing === s.query ? 'Removing…' : 'Remove'}
              </button>
            </div>
          </li>
        ))}
      </ul>
      {rows.length === 0 && !error && <p className="muted">No searches yet.</p>}

      {selected && (
        <section ref={cachedResultsRef} className="card cached-results">
          <div className="cached-results-header">
            <h2>{selected.query}</h2>
            <span className="muted">
              {selected.resultCount} saved · {formatWhen(selected.searchedAt)}
            </span>
            <Link className="btn-link" to={`/?q=${encodeURIComponent(selected.query)}`}>
              Search fresh
            </Link>
            <button
              type="button"
              className="text-btn danger"
              disabled={removing === selected.query}
              onClick={() => void removeOne(selected.query)}
            >
              {removing === selected.query ? 'Removing…' : 'Remove from history'}
            </button>
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
