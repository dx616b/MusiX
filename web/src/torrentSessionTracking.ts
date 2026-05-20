const active = new Set<string>()
let unloadInstalled = false

export function trackTorrentSession(infoHash: string | undefined): void {
  const ih = infoHash?.trim().toLowerCase()
  if (ih) active.add(ih)
}

export function untrackTorrentSession(infoHash: string | undefined): void {
  const ih = infoHash?.trim().toLowerCase()
  if (ih) active.delete(ih)
}

function beaconReleaseTorrentSessions(infoHashes: string[]): void {
  if (!infoHashes.length) return
  const url = '/api/torrent/sessions/release'
  const body = JSON.stringify({ infoHashes })
  if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
    const blob = new Blob([body], { type: 'application/json' })
    navigator.sendBeacon(url, blob)
    return
  }
  void fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
    keepalive: true,
  })
}

function releaseTrackedOnUnload(): void {
  const hashes = [...active]
  if (!hashes.length) return
  beaconReleaseTorrentSessions(hashes)
}

/** Register pagehide/beforeunload handlers once to release tracked torrent sessions. */
export function installTorrentSessionUnload(): void {
  if (unloadInstalled || typeof window === 'undefined') return
  unloadInstalled = true
  const handler = () => releaseTrackedOnUnload()
  window.addEventListener('pagehide', handler)
  window.addEventListener('beforeunload', handler)
}
