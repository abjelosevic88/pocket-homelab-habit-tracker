import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'

const nav = [
  { to: '/today', label: 'Today', ico: '◉' },
  { to: '/month', label: 'Month', ico: '▦' },
  { to: '/insights', label: 'Insights', ico: '◔' },
  { to: '/manage', label: 'Habits & trackers', ico: '☰' },
  { to: '/settings', label: 'Settings', ico: '⚙' },
]

export default function Layout() {
  const { user, settings, status, theme, toggleTheme, setUser } = useApp()
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const logout = async () => { await api.post(`${V1}/auth/logout`); setUser(null); navigate('/login') }
  const title = settings?.title || 'Pocket Habits'
  return (
    <div className="app">
      <aside className={`sidebar ${open ? 'open' : ''}`} onClick={() => setOpen(false)}>
        <div className="brand"><span className="logo">✓</span><span className="brand-name">{title}</span></div>
        <nav>
          {nav.map(n => <NavLink key={n.to} to={n.to} className={({ isActive }) => isActive ? 'active' : ''}><span className="ico">{n.ico}</span>{n.label}</NavLink>)}
        </nav>
        <div className="foot">
          <span className="foot-id" title={user?.email}><span>{user?.name || user?.email}</span>{status?.version && <a className="ver" href="https://github.com/abjelosevic88/pocket-homelab-habit-tracker/releases" target="_blank" rel="noreferrer" title="Version — click for release notes">v{status.version}</a>}</span>
          <span className="row">
            <button className="btn ghost sm" onClick={toggleTheme} title="Toggle theme">{theme === 'dark' ? '☀' : '☾'}</button>
            {status?.auth_mode === 'local' && <button className="btn ghost sm" onClick={logout} title="Sign out">⎋</button>}
          </span>
        </div>
      </aside>
      <div className="main">
        <header className="topbar">
          <button className="btn ghost icon menu-btn" onClick={() => setOpen(o => !o)} aria-label="Menu">☰</button>
          <span className="topbar-title">{title}</span>
          <span className="spacer" />
        </header>
        <main className="content"><Outlet /></main>
        <nav className="tabbar">
          {nav.slice(0, 4).map(n => <NavLink key={n.to} to={n.to} className={({ isActive }) => isActive ? 'active' : ''}><span className="ico">{n.ico}</span><span>{n.label.split(' ')[0]}</span></NavLink>)}
        </nav>
      </div>
    </div>
  )
}
