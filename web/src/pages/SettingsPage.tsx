import { FormEvent, useEffect, useState } from 'react'
import { getSettings, saveSettings, type AppSettings } from '../api'

export default function SettingsPage() {
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [prowlarrUrl, setProwlarrUrl] = useState('')
  const [prowlarrKey, setProwlarrKey] = useState('')
  const [prowlarrMusicCategories, setProwlarrMusicCategories] = useState('')
  const [jackettUrl, setJackettUrl] = useState('')
  const [jackettKey, setJackettKey] = useState('')
  const [jackettMusicCategories, setJackettMusicCategories] = useState('')
  const [transmissionUrl, setTransmissionUrl] = useState('')
  const [transmissionUser, setTransmissionUser] = useState('')
  const [transmissionPass, setTransmissionPass] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    void getSettings()
      .then((s) => {
        if (cancelled) return
        setSettings(s)
        setProwlarrUrl(s.prowlarr.url)
        setProwlarrMusicCategories((s.prowlarr.musicCategories ?? []).join(','))
        setJackettUrl(s.jackett.url)
        setJackettMusicCategories((s.jackett.musicCategories ?? []).join(','))
        setTransmissionUrl(s.transmission.url)
        setTransmissionUser(s.transmission.username)
        setProwlarrKey('')
        setJackettKey('')
        setTransmissionPass('')
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load settings')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  function parseCategoryCodes(raw: string) {
    const seen = new Set<string>()
    const out: string[] = []
    for (const part of raw.split(',')) {
      const code = part.trim()
      if (!code || seen.has(code)) continue
      seen.add(code)
      out.push(code)
    }
    return out
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSaved(false)
    try {
      const body = {
        prowlarr: {
          url: prowlarrUrl.trim(),
          ...(prowlarrKey.trim() ? { apiKey: prowlarrKey.trim() } : {}),
          musicCategories: parseCategoryCodes(prowlarrMusicCategories),
        },
        jackett: {
          url: jackettUrl.trim(),
          ...(jackettKey.trim() ? { apiKey: jackettKey.trim() } : {}),
          musicCategories: parseCategoryCodes(jackettMusicCategories),
        },
        transmission: {
          url: transmissionUrl.trim(),
          username: transmissionUser.trim(),
          ...(transmissionPass ? { password: transmissionPass } : {}),
        },
      }
      const next = await saveSettings(body)
      setSettings(next)
      setProwlarrKey('')
      setJackettKey('')
      setTransmissionPass('')
      setSaved(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="page">
      <h1>Settings</h1>
      <p className="muted">
        Configure indexers and Transmission. Changes apply immediately and are saved to{' '}
        <code>{settings?.overridePath ?? 'data/settings.yaml'}</code> (base config is read-only in Docker).
      </p>

      {loading && <p className="muted">Loading…</p>}
      {error && <p className="error">{error}</p>}
      {saved && <p className="muted">Settings saved.</p>}

      {!loading && (
        <form className="settings-form card" onSubmit={onSubmit}>
          <fieldset>
            <legend>Prowlarr</legend>
            <label>
              URL
              <input
                type="url"
                value={prowlarrUrl}
                onChange={(e) => setProwlarrUrl(e.target.value)}
                placeholder="https://prowlarr.example:9696"
              />
            </label>
            <label>
              API key
              <input
                type="password"
                value={prowlarrKey}
                onChange={(e) => setProwlarrKey(e.target.value)}
                placeholder={settings?.prowlarr.apiKeySet ? settings.prowlarr.apiKey ?? '••••••••' : 'Required when URL is set'}
                autoComplete="off"
              />
            </label>
            <label>
              Music category codes
              <input
                type="text"
                value={prowlarrMusicCategories}
                onChange={(e) => setProwlarrMusicCategories(e.target.value)}
                placeholder="3000,3010,3040"
              />
            </label>
          </fieldset>

          <fieldset>
            <legend>Jackett</legend>
            <label>
              URL
              <input
                type="url"
                value={jackettUrl}
                onChange={(e) => setJackettUrl(e.target.value)}
                placeholder="https://jackett.example:9117"
              />
            </label>
            <label>
              API key
              <input
                type="password"
                value={jackettKey}
                onChange={(e) => setJackettKey(e.target.value)}
                placeholder={settings?.jackett.apiKeySet ? settings.jackett.apiKey ?? '••••••••' : 'Required when URL is set'}
                autoComplete="off"
              />
            </label>
            <label>
              Music category codes
              <input
                type="text"
                value={jackettMusicCategories}
                onChange={(e) => setJackettMusicCategories(e.target.value)}
                placeholder="3000,3010,3040"
              />
            </label>
          </fieldset>

          <fieldset>
            <legend>Transmission</legend>
            <label>
              RPC URL
              <input
                type="url"
                value={transmissionUrl}
                onChange={(e) => setTransmissionUrl(e.target.value)}
                placeholder="http://127.0.0.1:9091/transmission/rpc"
              />
            </label>
            <label>
              Username
              <input
                type="text"
                value={transmissionUser}
                onChange={(e) => setTransmissionUser(e.target.value)}
                autoComplete="off"
              />
            </label>
            <label>
              Password
              <input
                type="password"
                value={transmissionPass}
                onChange={(e) => setTransmissionPass(e.target.value)}
                placeholder={settings?.transmission.passwordSet ? 'Leave blank to keep current' : 'Optional'}
                autoComplete="new-password"
              />
            </label>
          </fieldset>

          <p className="muted settings-hint">
            At least one of Prowlarr or Jackett must be configured. Leave API key / password blank to keep the existing value.
            Use music category codes like <code>3000,3010,3040</code>; leave empty to search all categories.
          </p>

          <button type="submit" disabled={saving}>
            {saving ? 'Saving…' : 'Save settings'}
          </button>
        </form>
      )}
    </div>
  )
}
