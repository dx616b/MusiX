import { FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import TorrentPreviewPanel from '../components/TorrentPreviewPanel'
import {
  formatBytes,
  getStoredSearch,
  listSearches,
  magnetForResult,
  queueDownload,
  search,
  type SearchHistory,
  type SearchResult,
} from '../api'

export default function SearchPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [query, setQuery] = useState(() => searchParams.get('q') ?? '')
  const [loading, setLoading] = useState(false)
  const [fromCache, setFromCache] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [results, setResults] = useState<SearchResult[]>([])
  const [history, setHistory] = useState<SearchHistory[]>([])
  const [queueing, setQueueing] = useState<string | null>(null)
  const [preview, setPreview] = useState<SearchResult | null>(null)
  const lastRan = useRef('')

  const loadHistory = useCallback(async () => {
    try {
      const data = await listSearches(12)
      setHistory(data.searches)
    } catch {
      /* optional sidebar */
    }
  }, [])

  const loadCached = useCallback(async (q: string) => {
    try {
      const row = await getStoredSearch(q)
      setResults(row.results ?? [])
      setFromCache(true)
      setError(null)
      return true
    } catch {
      return false
    }
  }, [])

  const executeSearch = useCallback(async (q: string) => {
    const trimmed = q.trim()
    if (!trimmed) return
    setLoading(true)
    setFromCache(false)
    setError(null)
    try {
      const data = await search(trimmed)
      setResults(data.results)
      await loadHistory()
    } catch (err) {
      setResults([])
      setError(err instanceof Error ? err.message : 'Search failed')
    } finally {
      setLoading(false)
    }
  }, [loadHistory])

  useEffect(() => {
    loadHistory()
  }, [loadHistory])

  useEffect(() => {
    const q = searchParams.get('q')?.trim() ?? ''
    const cached = searchParams.get('cached') === '1'
    setQuery(q)
    if (!q || q === lastRan.current) return
    lastRan.current = q

    if (cached) {
      setLoading(true)
      void loadCached(q).then((ok) => {
        if (!ok) void executeSearch(q)
        setLoading(false)
      })
      return
    }
    void executeSearch(q)
  }, [searchParams, executeSearch, loadCached])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    const trimmed = query.trim()
    if (!trimmed) return
    lastRan.current = trimmed
    setSearchParams({ q: trimmed })
    await executeSearch(trimmed)
  }

  async function onDownload(r: SearchResult) {
    const magnet = magnetForResult(r)
    if (!magnet) {
      setError('No magnet, info hash, or download URL for this release')
      return
    }
    setQueueing(r.title)
    setError(null)
    try {
      await queueDownload({
        query: query.trim(),
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
      <h1>Search music</h1>
      <p className="muted">Queries Prowlarr and Jackett for audio releases (FLAC, MP3, discography, …).</p>
      <form className="search-form" onSubmit={onSubmit}>
        <input
          type="search"
          placeholder="Artist, album, discography…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button type="submit" disabled={loading}>{loading ? 'Searching…' : 'Search'}</button>
      </form>
      {history.length > 0 && (
        <div className="recent-searches">
          <span className="muted">Recent:</span>
          {history.map((s) => (
            <button
              key={`${s.query}-${s.searchedAt}`}
              type="button"
              className="chip"
              disabled={loading}
              onClick={() => {
                lastRan.current = s.query
                setQuery(s.query)
                setSearchParams({ q: s.query, cached: '1' })
              }}
            >
              {s.query}
            </button>
          ))}
        </div>
      )}
      {fromCache && results.length > 0 && (
        <p className="muted cache-hint">
          Showing saved results.{' '}
          <button type="button" className="text-btn" onClick={() => void executeSearch(query)}>
            Search again
          </button>
        </p>
      )}
      {error && <p className="error">{error}</p>}
      <ul className="list">
        {results.map((r) => (
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
      {preview && (
        <TorrentPreviewPanel result={preview} query={query.trim()} onClose={() => setPreview(null)} />
      )}
    </div>
  )
}
