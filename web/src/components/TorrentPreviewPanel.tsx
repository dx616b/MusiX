import { useEffect, useState } from 'react'
import {
  ApiError,
  formatBytes,
  PREVIEW_METADATA_TIMEOUT_SECS,
  previewTorrent,
  torrentStreamUrl,
  type SearchResult,
  type TorrentPreview,
} from '../api'
import { usePlayer } from '../player/PlayerContext'

type Props = {
  result: SearchResult
  query?: string
  onClose: () => void
}

function FileRow({
  file,
  streamOpts,
  onPlay,
  active,
  paused,
}: {
  file: TorrentPreview['files'][0]
  streamOpts: {
    magnet?: string
    infoHash?: string
    downloadUrl?: string
    title: string
  }
  onPlay: () => void
  active: boolean
  paused: boolean
}) {
  return (
    <li className="row preview-file-row">
      <div className="result-meta">
        <span className="file-path">{file.path}</span>
        <span className="muted">{formatBytes(file.size)}</span>
        {active && (
          <span className="muted now-playing-badge">{paused ? 'Paused' : 'Playing'}</span>
        )}
      </div>
      <div className="row-actions">
        {file.audio && (
          <button type="button" className="btn-secondary" onClick={onPlay}>
            {active ? (paused ? 'Resume' : 'Pause') : 'Play'}
          </button>
        )}
        <a
          className="btn-link"
          href={torrentStreamUrl({ ...streamOpts, path: file.path, download: true })}
          download
        >
          Save
        </a>
      </div>
    </li>
  )
}

function displayTitle(preview: TorrentPreview | null, fallback: string) {
  const name = (preview?.name || fallback).trim()
  return name.length > 120 ? `${name.slice(0, 117)}…` : name
}

export default function TorrentPreviewPanel({ result, query: _query, onClose }: Props) {
  const { play, toggle, isActive, playing } = usePlayer()
  const [preview, setPreview] = useState<TorrentPreview | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [timedOut, setTimedOut] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setTimedOut(false)
    void previewTorrent({
      magnet: result.magnetUri,
      infoHash: result.infoHash,
      downloadUrl: result.downloadUrl,
      title: result.title,
    })
      .then((data) => {
        if (!cancelled) setPreview(data)
      })
      .catch((err) => {
        if (!cancelled) {
          if (err instanceof ApiError && err.isTimeout) {
            const secs = err.timeoutSecs ?? PREVIEW_METADATA_TIMEOUT_SECS
            setTimedOut(true)
            setError(`Timed out after ${secs}s — no metadata from the swarm. Try again or queue the download.`)
            return
          }
          setTimedOut(false)
          setError(err instanceof Error ? err.message : 'Preview failed')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [result])

  const streamOpts = {
    magnet: result.magnetUri,
    infoHash: preview?.infoHash || result.infoHash,
    downloadUrl: result.downloadUrl,
    title: result.title,
  }

  const audioFiles = preview?.files.filter((f) => f.audio) ?? []
  const otherFiles = preview?.files.filter((f) => !f.audio) ?? []

  function handlePlay(filePath: string, streamSrc: string) {
    if (isActive(streamSrc)) {
      toggle()
      return
    }
    play({
      src: streamSrc,
      torrentTitle: result.title,
      filePath,
    })
  }

  return (
    <div className="preview-overlay" role="dialog" aria-modal="true">
      <div className="preview-panel card">
        <div className="preview-header">
          <div className="preview-header-main">
            <h2 className="preview-title">{displayTitle(preview, result.title)}</h2>
            {preview && (
              <p className="muted preview-meta">
                {preview.fileCount} file{preview.fileCount === 1 ? '' : 's'} · {formatBytes(preview.totalSize)}
                {preview.truncated ? ' · truncated' : ''}
              </p>
            )}
          </div>
          <button type="button" className="btn-link" onClick={onClose}>
            Close
          </button>
        </div>

        {loading && (
          <p className="muted preview-status">Fetching files… (up to {PREVIEW_METADATA_TIMEOUT_SECS}s)</p>
        )}
        {error && (
          <p className={timedOut ? 'error preview-status preview-timeout' : 'error preview-status'}>
            {error}
          </p>
        )}

        {preview && (
          <div className="preview-body">
            {audioFiles.length > 0 && (
              <>
                <h3>Audio</h3>
                <ul className="list preview-files">
                  {audioFiles.map((f) => {
                    const streamSrc = torrentStreamUrl({ ...streamOpts, path: f.path })
                    const active = isActive(streamSrc)
                    return (
                      <FileRow
                        key={f.path}
                        file={f}
                        streamOpts={streamOpts}
                        active={active}
                        paused={active && !playing}
                        onPlay={() => handlePlay(f.path, streamSrc)}
                      />
                    )
                  })}
                </ul>
              </>
            )}
            {otherFiles.length > 0 && (
              <>
                <h3>Other</h3>
                <ul className="list preview-files">
                  {otherFiles.map((f) => (
                    <li key={f.path} className="row">
                      <span className="file-path">{f.path}</span>
                      <span className="muted">{formatBytes(f.size)}</span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
