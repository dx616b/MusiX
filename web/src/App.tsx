import { Link, Route, Routes, useNavigate } from 'react-router-dom'
import NowPlayingBar from './components/NowPlayingBar'
import SearchPage from './pages/SearchPage'
import SearchesPage from './pages/SearchesPage'
import DownloadsPage from './pages/DownloadsPage'
import SettingsPage from './pages/SettingsPage'
import { PlayerProvider, usePlayer } from './player/PlayerContext'

function AppShell() {
  const navigate = useNavigate()
  const { track } = usePlayer()
  return (
    <div className={`app${track ? ' has-player' : ''}`}>
      <header className="header">
        <button type="button" className="brand" onClick={() => navigate('/')}>
          MusiX
        </button>
        <nav className="nav">
          <Link to="/">Search</Link>
          <Link to="/searches">History</Link>
          <Link to="/downloads">Downloads</Link>
          <Link to="/settings">Settings</Link>
        </nav>
      </header>
      <main className="main">
        <Routes>
          <Route path="/" element={<SearchPage />} />
          <Route path="/searches" element={<SearchesPage />} />
          <Route path="/downloads" element={<DownloadsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </main>
      <NowPlayingBar />
    </div>
  )
}

export default function App() {
  return (
    <PlayerProvider>
      <AppShell />
    </PlayerProvider>
  )
}
