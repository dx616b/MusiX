import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
  const [musicOnly, setMusicOnly] = useState(() => searchParams.get('musicOnly') !== '0')
  const [loading, setLoading] = useState(false)
  const [fromCache, setFromCache] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [results, setResults] = useState<SearchResult[]>([])
  const [history, setHistory] = useState<SearchHistory[]>([])
  const [queueing, setQueueing] = useState<string | null>(null)
  const [preview, setPreview] = useState<SearchResult | null>(null)
  const [titleFilter, setTitleFilter] = useState('')
  const [indexerFilter, setIndexerFilter] = useState('all')
  const [minSeeders, setMinSeeders] = useState('')
  const [maxSizeGb, setMaxSizeGb] = useState('')
  const lastRan = useRef('')

  const loadHistory = useCallback(async () => {
    try {
      const data = await listSearches(12)
      setHistory(data.searches ?? [])
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

  const executeSearch = useCallback(async (q: string, musicMode: boolean) => {
    const trimmed = q.trim()
    if (!trimmed) return
    setLoading(true)
    setFromCache(false)
    setError(null)
    try {
      const data = await search(trimmed, musicMode)
      setResults(data.results ?? [])
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
    const musicParam = searchParams.get('musicOnly')
    const nextMusicOnly = musicParam !== '0'
    setQuery(q)
    setMusicOnly(nextMusicOnly)
    if (!q || q === lastRan.current) return
    lastRan.current = q

    if (cached) {
      setLoading(true)
      void loadCached(q).then((ok) => {
        if (!ok) void executeSearch(q, nextMusicOnly)
        setLoading(false)
      })
      return
    }
    void executeSearch(q, nextMusicOnly)
  }, [searchParams, executeSearch, loadCached])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    const trimmed = query.trim()
    if (!trimmed) return
    lastRan.current = trimmed
    setSearchParams({ q: trimmed, musicOnly: musicOnly ? '1' : '0' })
    await executeSearch(trimmed, musicOnly)
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

  const indexers = useMemo(() => {
    return Array.from(new Set(results.map((r) => r.indexer).filter(Boolean))).sort((a, b) =>
      a.localeCompare(b),
    )
  }, [results])

  const filteredResults = useMemo(() => {
    const text = titleFilter.trim().toLowerCase()
    const min = Number(minSeeders)
    const maxBytes = Number(maxSizeGb) * 1024 * 1024 * 1024

    return results.filter((r) => {
      if (indexerFilter !== 'all' && r.indexer !== indexerFilter) return false
      if (text && !`${r.title} ${r.indexer}`.toLowerCase().includes(text)) return false
      if (!Number.isNaN(min) && minSeeders.trim() !== '' && r.seeders < min) return false
      if (!Number.isNaN(maxBytes) && maxSizeGb.trim() !== '' && r.size > maxBytes) return false
      return true
    })
  }, [results, titleFilter, indexerFilter, minSeeders, maxSizeGb])

  function clearFilters() {
    setTitleFilter('')
    setIndexerFilter('all')
    setMinSeeders('')
    setMaxSizeGb('')
  }

  return (
    <div className="page">
      <h1>Search torrents</h1>
      <p className="muted">Search Prowlarr and Jackett. Toggle music-only mode as needed.</p>
      <form className="search-form" onSubmit={onSubmit}>
        <input
          type="search"
          placeholder="Artist, album, discography…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <label className="search-toggle">
          <input
            type="checkbox"
            checked={musicOnly}
            onChange={(e) => setMusicOnly(e.target.checked)}
          />
          Music only
        </label>
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
                setSearchParams({ q: s.query, cached: '1', musicOnly: musicOnly ? '1' : '0' })
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
          <button type="button" className="text-btn" onClick={() => void executeSearch(query, musicOnly)}>
            Search again
          </button>
        </p>
      )}
      {error && <p className="error">{error}</p>}
      {results.length > 0 && (
        <div className="card filters-panel">
          <div className="filters-head">
            <strong>Filters</strong>
            <button type="button" className="text-btn" onClick={clearFilters}>
              Clear
            </button>
          </div>
          <div className="filters-grid">
            <label>
              <span className="muted">Title or indexer</span>
              <input
                type="text"
                value={titleFilter}
                onChange={(e) => setTitleFilter(e.target.value)}
                placeholder="Filter results..."
              />
            </label>
            <label>
              <span className="muted">Indexer</span>
              <select value={indexerFilter} onChange={(e) => setIndexerFilter(e.target.value)}>
                <option value="all">All indexers</option>
                {indexers.map((indexer) => (
                  <option key={indexer} value={indexer}>
                    {indexer}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span className="muted">Min seeders</span>
              <input
                type="number"
                min={0}
                step={1}
                value={minSeeders}
                onChange={(e) => setMinSeeders(e.target.value)}
                placeholder="0"
              />
            </label>
            <label>
              <span className="muted">Max size (GB)</span>
              <input
                type="number"
                min={0}
                step={0.1}
                value={maxSizeGb}
                onChange={(e) => setMaxSizeGb(e.target.value)}
                placeholder="Any"
              />
            </label>
          </div>
          <p className="muted filters-summary">
            Showing {filteredResults.length} of {results.length} results
          </p>
        </div>
      )}
      <ul className="list">
        {filteredResults.map((r) => (
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
      {results.length > 0 && filteredResults.length === 0 && (
        <p className="muted">No results match the current filters.</p>
      )}
      {preview && (
        <TorrentPreviewPanel result={preview} query={query.trim()} onClose={() => setPreview(null)} />
      )}
    </div>
  )
}
