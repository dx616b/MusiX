import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from 'react'
import { infoHashFromStreamUrl } from '../api'
import {
  installTorrentSessionUnload,
  trackTorrentSession,
  untrackTorrentSession,
} from '../torrentSessionTracking'
export type NowPlayingTrack = {
  src: string
  torrentTitle: string
  filePath: string
}

type PlayerContextValue = {
  track: NowPlayingTrack | null
  playing: boolean
  audioRef: RefObject<HTMLAudioElement>
  play: (track: NowPlayingTrack) => void
  toggle: () => void
  stop: () => void
  isActive: (src: string) => boolean
}

const PlayerContext = createContext<PlayerContextValue | null>(null)

export function PlayerProvider({ children }: { children: ReactNode }) {
  const audioRef = useRef<HTMLAudioElement>(null)
  const [track, setTrack] = useState<NowPlayingTrack | null>(null)
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
    installTorrentSessionUnload()
  }, [])

  useEffect(() => {
    const ih = track ? infoHashFromStreamUrl(track.src) : undefined
    if (!ih) return
    trackTorrentSession(ih)
    return () => untrackTorrentSession(ih)
  }, [track?.src])

  const stop = useCallback(() => {
    const el = audioRef.current
    if (el) {
      el.pause()
      el.removeAttribute('src')
      el.load()
    }
    setTrack(null)
    setPlaying(false)
  }, [])

  const play = useCallback((next: NowPlayingTrack) => {
    setTrack(next)
    setPlaying(true)
  }, [])

  const toggle = useCallback(() => {
    setPlaying((p) => !p)
  }, [])

  const isActive = useCallback(
    (src: string) => track?.src === src,
    [track],
  )

  useEffect(() => {
    const el = audioRef.current
    if (!el || !track) return

    const absolute = track.src.startsWith('http')
      ? track.src
      : new URL(track.src, window.location.origin).href

    if (el.src !== absolute) {
      el.src = track.src
    }

    if (playing) {
      void el.play().catch(() => setPlaying(false))
    } else {
      el.pause()
    }
  }, [track, playing])

  useEffect(() => {
    const el = audioRef.current
    if (!el) return
    const onEnded = () => {
      setPlaying(false)
    }
    el.addEventListener('ended', onEnded)
    return () => el.removeEventListener('ended', onEnded)
  }, [track])

  return (
    <PlayerContext.Provider value={{ track, playing, audioRef, play, toggle, stop, isActive }}>
      {children}
    </PlayerContext.Provider>
  )
}

export function usePlayer() {
  const ctx = useContext(PlayerContext)
  if (!ctx) {
    throw new Error('usePlayer must be used within PlayerProvider')
  }
  return ctx
}
