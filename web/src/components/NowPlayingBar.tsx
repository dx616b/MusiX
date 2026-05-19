import { usePlayer } from '../player/PlayerContext'

function fileLabel(path: string) {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

export default function NowPlayingBar() {
  const { track, playing, audioRef, toggle, stop } = usePlayer()

  if (!track) return null

  return (
    <div className="now-playing-bar" role="region" aria-label="Now playing">
      <div className="now-playing-inner">
        <div className="now-playing-top">
          <div className="now-playing-meta">
            <strong className="now-playing-title">{track.torrentTitle}</strong>
            <span className="muted now-playing-file">{fileLabel(track.filePath)}</span>
          </div>
          <div className="now-playing-controls">
            <button type="button" className="btn-secondary" onClick={toggle}>
              {playing ? 'Pause' : 'Play'}
            </button>
            <button type="button" className="btn-secondary" onClick={stop}>
              Stop
            </button>
          </div>
        </div>
        <audio
          ref={audioRef}
          className="now-playing-audio"
          controls
          preload="metadata"
          playsInline
        />
      </div>
    </div>
  )
}
