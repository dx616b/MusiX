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

/** Default torrent metadata timeout (seconds); matches server unless TORRENT_MAGNET_METADATA_TIMEOUT_SECS is set. */
export const PREVIEW_METADATA_TIMEOUT_SECS = 90

export function search(q: string) {
  return get<{ query: string; results: SearchResult[] }>(`/api/search?q=${encodeURIComponent(q)}`)
}

export function listSearches(limit = 50, includeResults = false) {
  const params = new URLSearchParams({ limit: String(limit) })
  if (includeResults) params.set('includeResults', 'true')
  return get<{ searches: SearchHistory[] }>(`/api/searches?${params}`)
}

export function getStoredSearch(q: string) {
  return get<SearchHistory>(`/api/searches?q=${encodeURIComponent(q)}`)
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

export function torrentStreamUrl(opts: {
  magnet?: string
  infoHash?: string
  downloadUrl?: string
  title: string
  path: string
  download?: boolean
}) {
  const params = new URLSearchParams({ title: opts.title, path: opts.path })
  const magnet = opts.magnet || opts.downloadUrl || ''
  if (magnet) params.set('magnet', magnet)
  if (opts.infoHash) params.set('infoHash', opts.infoHash)
  if (opts.download) params.set('download', '1')
  return `/api/torrent/stream?${params}`
}

export function previewTorrent(opts: {
  magnet?: string
  infoHash?: string
  downloadUrl?: string
  title: string
}) {
  const params = new URLSearchParams({ title: opts.title })
  const magnet = opts.magnet || opts.downloadUrl || ''
  if (magnet) params.set('magnet', magnet)
  if (opts.infoHash) params.set('infoHash', opts.infoHash)
  return get<TorrentPreview>(`/api/torrent/preview?${params}`)
}

export function magnetForResult(r: SearchResult): string {
  return r.magnetUri || (r.infoHash ? `magnet:?xt=urn:btih:${r.infoHash}` : '') || r.downloadUrl || ''
}

export interface AppSettings {
  configPath: string
  overridePath: string
  prowlarr: { url: string; apiKeySet: boolean; apiKey?: string }
  jackett: { url: string; apiKeySet: boolean; apiKey?: string }
  transmission: { url: string; username: string; passwordSet: boolean }
}

export interface AppSettingsUpdate {
  prowlarr?: { url: string; apiKey?: string }
  jackett?: { url: string; apiKey?: string }
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
