import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { GeoPlace, APIToken, Settings as SettingsT } from '../lib/types'
import { Avatar, Confirm, Field, Loading, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { ShortcutsSheet } from '../components/ShortcutsSheet'

type TabId = 'general' | 'reminders' | 'weather' | 'profile' | 'api' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'The journal prompts, the page the app opens on, and the app title.' },
  { id: 'reminders', label: 'Reminder', icon: 'bell', sub: 'An evening push when habits are still unmarked or the journal is empty.' },
  { id: 'weather', label: 'Weather', icon: 'sun', sub: "The day's weather next to your habits and scores." },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'api', label: 'API tokens', icon: 'key', sub: 'Tokens for Homepage widgets, iOS Shortcuts, Home Assistant and n8n.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Download the database, export or import your notebook.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]
// old and obvious names (?tab=… or /settings/<name>)
const ALIAS: Record<string, TabId> = { reminder: 'reminders', tokens: 'api', backup: 'data' }

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const [shortcuts, setShortcuts] = useState(false)
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  const want = raw || legacy
  const tab: TabId = (TABS.find(t => t.id === (ALIAS[want] || want))?.id) || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })

  // an old ?tab= link lands on /settings/<tab>
  useEffect(() => { if (!raw && legacy) navigate(`/settings/${tab}`, { replace: true }) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back to the notebook" title="Back to the notebook (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{current.label}</h2>
              <p>{current.sub}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'reminders' && <Reminders />}
            {tab === 'weather' && <Weather />}
            {tab === 'profile' && <Profile />}
            {tab === 'api' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About onShortcuts={() => setShortcuts(true)} />}
          </div>
        </main>
      </div>
      {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
    </div>
  )
}

/** A settings card: title, explanation, actions on the right — Pocket Calendar's section. */
function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  // Settings may arrive after the tab mounted (direct load of /settings): adopt them once.
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (override?: SettingsT, msg = 'Saved') => {
    const body = override ?? f
    if (!body) return false
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, body); setSettings(s); setF(s); toast(msg); return true } catch (e) { toast((e as Error).message, 'err'); return false } finally { setBusy(false) }
  }
  const dirty = Boolean(f && settings && JSON.stringify(f) !== JSON.stringify(settings))
  return { f, setF, save, busy, dirty }
}

function SaveButton({ dirty, busy, onSave, disabled }: { dirty: boolean; busy: boolean; onSave: () => void; disabled?: boolean }) {
  return <button type="button" className="btn primary" onClick={onSave} disabled={busy || disabled}>{dirty ? 'Save changes' : 'Save'}</button>
}

function General() {
  const { f, setF, save, busy, dirty } = useSettingsForm()
  const { user } = useApp()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="The notebook" sub="Yours alone — everyone on the box has their own.">
        <SettingRow title="Journal prompt" help="The question above the one good thing of the day."><input value={f.journal_prompt} onChange={e => setF({ ...f, journal_prompt: e.target.value })} maxLength={80} /></SettingRow>
        <SettingRow title="Second line: one bad thing" help="Adds a second box on the Today page and a second column on the month page. Off by default — the notebook only asks for the good thing.">
          <Switch checked={f.bad_thing_enabled} onChange={v => setF({ ...f, bad_thing_enabled: v })} label="Second line: one bad thing" />
        </SettingRow>
        {f.bad_thing_enabled && <SettingRow title="Bad-thing prompt"><input value={f.bad_prompt} onChange={e => setF({ ...f, bad_prompt: e.target.value })} maxLength={80} /></SettingRow>}
        <SettingRow title="Open on" help="The page the app shows first."><Segmented size="sm" value={f.landing} onChange={v => setF({ ...f, landing: v })} options={[{ value: 'month', label: 'Month page' }, { value: 'today', label: 'Today check-in' }]} /></SettingRow>
      </Section>
      {user?.role === 'admin' && (
        <Section title="This box" sub="Applies to everyone on this box.">
          <SettingRow title="App title" help="Shown in the sidebar and the browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} /></SettingRow>
        </Section>
      )}
      <div className="row"><SaveButton dirty={dirty} busy={busy} onSave={() => save()} /></div>
    </div>
  )
}

function Reminders() {
  const { f, setF, save, busy, dirty } = useSettingsForm()
  const { toast } = useToast()
  const [testing, setTesting] = useState(false)
  if (!f) return <Loading />
  const test = async () => { setTesting(true); try { await api.post(`${V1}/settings/test-reminder`, f); toast('Test sent') } catch (e) { toast((e as Error).message, 'err') } finally { setTesting(false) } }
  return (
    <div className="stack loose">
      <Section title="Evening reminder" icon="bell" sub="Once a day, if habits are still unmarked (or the journal is empty), a push is sent. Works with ntfy, Apprise, or any webhook. Leave the time empty to turn it off.">
        <SettingRow title="Time" help="Server local time."><input type="time" className="set-time" value={f.reminder_time} onChange={e => setF({ ...f, reminder_time: e.target.value })} /></SettingRow>
        <SettingRow title="Format"><Segmented size="sm" value={f.reminder_format} onChange={v => setF({ ...f, reminder_format: v })} options={[{ value: 'ntfy', label: 'ntfy' }, { value: 'apprise', label: 'Apprise' }, { value: 'json', label: 'JSON webhook' }]} /></SettingRow>
        <SettingRow stack title="Endpoint" help={<>ntfy: <code>https://ntfy.sh/your-topic</code> · Apprise: <code>http://apprise:8000/notify/habits</code> · json: any URL that accepts a POST</>}>
          <input value={f.reminder_url} onChange={e => setF({ ...f, reminder_url: e.target.value })} placeholder="https://ntfy.example.com/habits" />
        </SettingRow>
      </Section>
      <div className="row">
        <SaveButton dirty={dirty} busy={busy} onSave={() => save()} />
        <button type="button" className="btn" onClick={test} disabled={testing || !f.reminder_url}><Icon name="bell" size={15} />{testing ? 'Sending…' : 'Send test'}</button>
      </div>
    </div>
  )
}

function Weather() {
  const { f, setF, save, busy, dirty } = useSettingsForm()
  const { settings } = useApp()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const [places, setPlaces] = useState<GeoPlace[]>([])
  const [searching, setSearching] = useState(false)
  const [fetching, setFetching] = useState(false)
  const [days, setDays] = useState<number | null>(null)
  if (!f) return <Loading />
  const search = async () => {
    if (q.trim().length < 2) return
    setSearching(true)
    try { setPlaces(await api.get<GeoPlace[]>(`${V1}/weather/geocode?q=${encodeURIComponent(q.trim())}`)) } catch (e) { toast((e as Error).message, 'err') } finally { setSearching(false) }
  }
  const fetchNow = async () => {
    if (dirty && !(await save())) return
    setFetching(true)
    try { const r = await api.post<{ days: number }>(`${V1}/weather/refresh`, {}); setDays(r.days); toast(`Fetched ${r.days} days`) } catch (e) { toast((e as Error).message, 'err') } finally { setFetching(false) }
  }
  const hasHome = f.weather_lat !== 0 || f.weather_lon !== 0
  const unsavedHome = hasHome && (f.weather_lat !== settings?.weather_lat || f.weather_lon !== settings?.weather_lon)
  // Picking a town saves it right away, so a reload or "Fetch now" never loses it.
  const pick = async (p: GeoPlace) => { const next = { ...f, weather_place: p.label, weather_lat: p.lat, weather_lon: p.lon }; setF(next); setPlaces([]); setQ(''); await save(next, `Home location saved: ${p.name || p.label}`) }
  return (
    <div className="stack loose">
      <Section title="Weather" icon="sun" sub={<>The day's weather from <a href="https://open-meteo.com/" target="_blank" rel="noreferrer">Open-Meteo</a> (free, no key): the condition, high and low, rain, sunshine and daylight, for your home location — or for where your phone was, when the companion app sends its location once a day (<code>POST /api/v1/location</code>). It shows on the month page and the check-in, and Insights compares your scores by weather, temperature and daylight.</>}>
        <SettingRow stack title="Home location" help={hasHome ? undefined : 'Type a town and pick it from the list.'}>
          {hasHome && (
            <div className={`callout ${unsavedHome ? 'success' : ''} geo-home`}>
              <Icon name="pin" size={17} className="geo-pin" />
              <span className="grow"><b>{f.weather_place || 'Home'}</b> <span className="muted small">{f.weather_lat.toFixed(3)}, {f.weather_lon.toFixed(3)}</span>{unsavedHome && <span className="muted small"> · not saved yet</span>}</span>
              <button type="button" className="btn ghost sm" onClick={() => setF({ ...f, weather_place: '', weather_lat: 0, weather_lon: 0, weather_enabled: false })}>Change</button>
            </div>
          )}
          {!hasHome && <div className="row"><input value={q} onChange={e => setQ(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); search() } }} placeholder="Town or city" /><button type="button" className="btn" onClick={search} disabled={searching}><Icon name="search" size={15} />{searching ? 'Searching…' : 'Search'}</button></div>}
          {places.length > 0 && (
            <ul className="geo-list">
              {places.map(p => <li key={`${p.lat},${p.lon}`}><button type="button" className="btn ghost" onClick={() => pick(p)}><span className="ellipsis">{p.label}</span> <span className="muted small">{p.lat.toFixed(2)}, {p.lon.toFixed(2)}</span></button></li>)}
            </ul>
          )}
        </SettingRow>
        <SettingRow title="Fetch the weather" help={f.weather_enabled && !hasHome ? 'Pick a home location first.' : undefined}><Switch checked={f.weather_enabled} onChange={v => setF({ ...f, weather_enabled: v })} label="Fetch the weather" /></SettingRow>
        <SettingRow title="Temperatures in"><Segmented size="sm" value={f.weather_units} onChange={v => setF({ ...f, weather_units: v })} options={[{ value: 'celsius', label: '°C' }, { value: 'fahrenheit', label: '°F' }]} /></SettingRow>
      </Section>
      <div className="row wrap">
        <SaveButton dirty={dirty} busy={busy} onSave={() => save()} disabled={f.weather_enabled && !hasHome} />
        <button type="button" className="btn" onClick={fetchNow} disabled={fetching || !f.weather_enabled}><Icon name="refresh" size={15} />{fetching ? 'Fetching…' : 'Fetch now'}</button>
        {days !== null && <span className="muted small">{days} days written</span>}
        {f.weather_last && <span className="muted small">last fetch {f.weather_last.slice(0, 16).replace('T', ' ')}</span>}
      </div>
      <p className="muted small">Turning it on fetches the last six weeks at once and the rest since your first habit in the background; the scheduler refreshes today and yesterday every three hours. A day's condition can be changed by hand on the month page.</p>
    </div>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoComplete="name" /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} autoComplete="email" /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && (
        <Section title="Password" sub="Leave empty to keep the current one.">
          <div className="stack">
            <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
            <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
          </div>
        </Section>
      )}
      <div className="row"><button type="submit" className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. Good for Homepage widgets (<code>GET /api/v1/dashboard</code>), iOS Shortcuts (<code>POST /api/v1/checkin</code>), Home Assistant and n8n. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button type="submit" className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? t.last_used_at.slice(0, 16).replace('T', ' ') : 'never'}</td><td className="muted">{t.created_at.slice(0, 10)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
    </div>
  )
}

function Data() {
  const { data: sys } = useAsync(() => api.get<{ db_download?: boolean }>(`${V1}/settings/system`), [])
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const importFile = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<{ created: number; entries: number }>(`${V1}/import`, JSON.parse(await file.text())); toast(`Imported ${r.entries} entries (${r.created} new habits/trackers)`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose">
      {sys?.db_download && <Section title="Database backup" icon="database" sub="A consistent copy of the SQLite database (VACUUM INTO).">
        <p className="muted">Restore by stopping the app and replacing <code>pocket-habits.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}><Icon name="download" size={15} />Download database</a>
      </Section>}
      <Section title="Export & import" icon="archive" sub="Your own habits, trackers and journal as portable JSON (re-importable here) or a long CSV for spreadsheets.">
        <div className="row wrap mb"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Export CSV</a></div>
        <label className="btn ghost"><Icon name="upload" size={15} />{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importFile(e.target.files[0])} /></label>
        <p className="muted small mt">Import merges: habits and trackers are matched by name, entries are upserted.</p>
      </Section>
    </div>
  )
}

function About({ onShortcuts }: { onShortcuts: () => void }) {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const tiles: [string, number | undefined, string][] = [['Habits', counts.habits, 'check-square'], ['Marks', counts.habit_entries, 'check'], ['Tracker values', counts.tracker_entries, 'sliders'], ['Journal lines', counts.journal, 'note']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
  ]
  if (data.auth_mode !== undefined) rows.push(['Sign-in', String(data.auth_mode)])
  if (data.db_path !== undefined) rows.push(['Database', <span key="db"><code>{String(data.db_path)}</code> · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</span>])
  if (data.counts !== undefined) rows.push(['Rows', `${counts.habits ?? 0} habits · ${counts.habit_entries ?? 0} marks · ${counts.trackers ?? 0} trackers · ${counts.tracker_entries ?? 0} values · ${counts.journal ?? 0} journal lines`])
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Habits</h3>
          <p className="muted small">A one-page habit tracker modelled on a bullet-journal spread: seven habits in green and red, a sleep score and day rating out of five, a weight row, and one good thing per day. One binary, one SQLite file, one volume. Inspired by the “I Finally Made Habit Tracking Simple” notebook system.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{n ?? 0}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={onShortcuts}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-habit-tracker" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Open source (MIT) on GitHub</a>
      </div>
    </div>
  )
}
