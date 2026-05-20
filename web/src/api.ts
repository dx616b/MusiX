export interface SearchResult {
  title: string
  size: number
  seeders: number
  peers: number
  indexer: string
  magnetUri?: string
  infoHash?: string
  downloadUrl?: string
}

export interface SearchHistory {
  query: string
  resultCount: number
  searchedAt: string
  results?: SearchResult[]
}

export interface Download {
  id: number
  query?: string
  title: string
  magnet: string
  infoHash?: string
  indexer?: string
  status: string
  transmissionId?: number
  percentDone: number
  createdAt: string
  updatedAt: string
}

export type ApiErrorBody = {
  error?: string
  message?: string
  timeoutSecs?: number
}

export class ApiError extends Error {
  code?: string
  timeoutSecs?: number

  constructor(body: ApiErrorBody, fallback: string) {
    super(body.message || fallback)
    this.name = 'ApiError'
    this.code = body.error
    this.timeoutSecs = body.timeoutSecs
  }

  get isTimeout() {
    return this.code === 'timeout'
  }
}

async function readApiError(res: Response): Promise<ApiError> {
  const text = await res.text()
  try {
    const body = JSON.parse(text) as ApiErrorBody
    if (body.error || body.message) {
      return new ApiError(body, text || res.statusText)
    }
  } catch {
    /* plain text */
  }
  return new ApiError({ message: text }, text || res.statusText)
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw await readApiError(res)
  return res.json()
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw await readApiError(res)
  return res.json()
}

async function httpDelete(path: string): Promise<unknown> {
  const res = await fetch(path, { method: 'DELETE' })
  if (!res.ok) throw await readApiError(res)
  const text = await res.text()
  if (!text.trim()) return undefined
  try {
    return JSON.parse(text) as unknown
  } catch {
    return text
  }
}

/** Default torrent metadata timeout (seconds); matches server unless TORRENT_MAGNET_METADATA_TIMEOUT_SECS is set. */
export const PREVIEW_METADATA_TIMEOUT_SECS = 90

export function search(q: string, musicOnly = true) {
  const params = new URLSearchParams({ q })
  params.set('musicOnly', musicOnly ? '1' : '0')
  return get<{ query: string; musicOnly: boolean; results: SearchResult[] }>(`/api/search?${params}`)
}

/** Match server-side search key normalization (store.normalizeSearchQuery). */
export function normalizeSearchQuery(q: string): string {
  return q
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .join(' ')
}

export function listSearches(limit = 50, includeResults = false) {
  const params = new URLSearchParams({ limit: String(limit) })
  if (includeResults) params.set('includeResults', 'true')
  return get<{ searches: SearchHistory[] }>(`/api/searches?${params}`)
}

export function getStoredSearch(q: string) {
  return get<SearchHistory>(`/api/searches?q=${encodeURIComponent(q)}`)
}

/** Remove one saved search by query text. */
export function deleteStoredSearch(q: string) {
  return httpDelete(`/api/searches?q=${encodeURIComponent(q)}`) as Promise<{ deleted: boolean; query: string }>
}

/** Remove all saved searches. Returns count cleared. */
export function clearAllStoredSearches() {
  return httpDelete('/api/searches?all=1') as Promise<{ cleared: number }>
}

export function listDownloads() {
  return get<{ downloads: Download[] }>('/api/downloads')
}

export function queueDownload(payload: {
  query?: string
  title: string
  magnet: string
  infoHash?: string
  indexer?: string
}) {
  return post<Download>('/api/downloads', payload)
}

export interface TorrentPreviewFile {
  path: string
  size: number
  audio: boolean
}

export interface TorrentPreview {
  name: string
  infoHash: string
  totalSize: number
  fileCount: number
  truncated?: boolean
  files: TorrentPreviewFile[]
}

/** Parse btih from a magnet URI so we can avoid putting huge tracker lists in query strings. */
export function parseInfoHashFromMagnet(magnet: string): string | undefined {
  const raw = magnet.trim()
  if (!raw.toLowerCase().startsWith('magnet:')) return undefined
  try {
    const u = new URL(raw)
    const xt = u.searchParams.get('xt') ?? ''
    const prefix = 'urn:btih:'
    if (!xt.toLowerCase().startsWith(prefix)) return undefined
    const hash = xt.slice(prefix.length).split('&')[0].trim()
    return hash ? hash.toLowerCase() : undefined
  } catch {
    const m = raw.match(/btih:([a-fA-F0-9]{40})/i)
    return m?.[1]?.toLowerCase()
  }
}

function effectiveTorrentInfoHash(opts: {
  infoHash?: string
  magnet?: string
}): string | undefined {
  const ih = opts.infoHash?.trim().toLowerCase()
  if (ih) return ih
  if (opts.magnet) return parseInfoHashFromMagnet(opts.magnet)
  return undefined
}

/** Query params for preview/stream; prefers infoHash so reverse proxies do not reject oversized URLs. */
function torrentQueryParams(opts: {
  title: string
  path?: string
  magnet?: string
  infoHash?: string
  downloadUrl?: string
  download?: boolean
}): URLSearchParams {
  const params = new URLSearchParams({ title: opts.title })
  if (opts.path) params.set('path', opts.path)
  const ih = effectiveTorrentInfoHash(opts)
  if (ih) {
    params.set('infoHash', ih)
  } else {
    const magnet = opts.magnet?.trim() || opts.downloadUrl?.trim() || ''
    if (magnet) params.set('magnet', magnet)
  }
  if (opts.download) params.set('download', '1')
  return params
}

export function torrentStreamUrl(opts: {
  magnet?: string
  infoHash?: string
  downloadUrl?: string
  title: string
  path: string
  download?: boolean
}) {
  return `/api/torrent/stream?${torrentQueryParams(opts)}`
}

export function previewTorrent(opts: {
  magnet?: string
  infoHash?: string
  downloadUrl?: string
  title: string
}) {
  return get<TorrentPreview>(`/api/torrent/preview?${torrentQueryParams(opts)}`)
}

export function magnetForResult(r: SearchResult): string {
  return r.magnetUri || (r.infoHash ? `magnet:?xt=urn:btih:${r.infoHash}` : '') || r.downloadUrl || ''
}

export interface AppSettings {
  configPath: string
  overridePath: string
  prowlarr: { url: string; apiKeySet: boolean; apiKey?: string; musicCategories?: string[] }
  jackett: { url: string; apiKeySet: boolean; apiKey?: string; musicCategories?: string[] }
  transmission: { url: string; username: string; passwordSet: boolean }
}

export interface AppSettingsUpdate {
  prowlarr?: { url: string; apiKey?: string; musicCategories?: string[] }
  jackett?: { url: string; apiKey?: string; musicCategories?: string[] }
  transmission?: { url: string; username?: string; password?: string }
}

export function getSettings() {
  return get<AppSettings>('/api/settings')
}

export function saveSettings(body: AppSettingsUpdate) {
  return put<AppSettings>('/api/settings', body)
}

async function put<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export function formatBytes(n: number) {
  if (!n) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}
