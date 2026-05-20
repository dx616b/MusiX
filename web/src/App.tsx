import { NavLink, Route, Routes, useNavigate } from 'react-router-dom'
import NowPlayingBar from './components/NowPlayingBar'
import SearchPage from './pages/SearchPage'
import SearchesPage from './pages/SearchesPage'
import DownloadsPage from './pages/DownloadsPage'
import SettingsPage from './pages/SettingsPage'
import { PlayerProvider, usePlayer } from './player/PlayerContext'

function navClass({ isActive }: { isActive: boolean }) {
  return isActive ? 'nav-btn is-active' : 'nav-btn'
}

function AppShell() {
  const navigate = useNavigate()
  const { track } = usePlayer()
  return (
    <div className={`app${track ? ' has-player' : ''}`}>
      <header className="header">
        <button type="button" className="brand" onClick={() => navigate('/')}>
          MusiX
        </button>
        <nav className="nav" aria-label="Main">
          <NavLink to="/" end className={navClass}>
            Search
          </NavLink>
          <NavLink to="/searches" className={navClass}>
            History
          </NavLink>
          <NavLink to="/downloads" className={navClass}>
            Downloads
          </NavLink>
          <NavLink to="/settings" className={navClass}>
            Settings
          </NavLink>
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
