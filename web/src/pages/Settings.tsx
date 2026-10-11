import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { GeoPlace, APIToken, Settings as SettingsT } from '../lib/types'
import { Card, Confirm, Field, Loading, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'

type Tab = 'general' | 'reminders' | 'weather' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>('general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'reminders', label: 'Reminder' }, { id: 'weather', label: 'Weather' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API tokens' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'reminders' && <Reminders />}
        {tab === 'weather' && <Weather />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
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

function General() {
  const { f, setF, save, busy } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <Card title="Preferences">
      <div className="grid" style={{ gap: 14, maxWidth: 560 }}>
        <Field label="App title" help="Shown in the sidebar and browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} /></Field>
        <Field label="Journal prompt"><input value={f.journal_prompt} onChange={e => setF({ ...f, journal_prompt: e.target.value })} maxLength={80} /></Field>
        <Field label="Second line: one bad thing" help="Adds a second box on the Today page and a second column on the month page. Off by default — the notebook only asks for the good thing.">
          <Segmented value={f.bad_thing_enabled ? 'on' : 'off'} onChange={v => setF({ ...f, bad_thing_enabled: v === 'on' })} options={[{ value: 'off', label: 'Off' }, { value: 'on', label: 'On' }]} />
        </Field>
        {f.bad_thing_enabled && <Field label="Bad-thing prompt"><input value={f.bad_prompt} onChange={e => setF({ ...f, bad_prompt: e.target.value })} maxLength={80} /></Field>}
        <Field label="Open on"><Segmented value={f.landing} onChange={v => setF({ ...f, landing: v })} options={[{ value: 'month', label: 'Month page' }, { value: 'today', label: 'Today check-in' }]} /></Field>
        <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div>
      </div>
    </Card>
  )
}

function Reminders() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [testing, setTesting] = useState(false)
  if (!f) return <Loading />
  const test = async () => { setTesting(true); try { await api.post(`${V1}/settings/test-reminder`, f); toast('Test sent') } catch (e) { toast((e as Error).message, 'err') } finally { setTesting(false) } }
  return (
    <Card title="Evening reminder">
      <p className="muted">Once a day, if habits are still unmarked (or the journal is empty), a push is sent. Works with ntfy, Apprise, or any webhook. Leave the time empty to turn it off.</p>
      <div className="grid" style={{ gap: 14, maxWidth: 560 }}>
        <Field label="Time (server local time)"><input type="time" value={f.reminder_time} onChange={e => setF({ ...f, reminder_time: e.target.value })} /></Field>
        <Field label="Endpoint" help="ntfy: https://ntfy.sh/your-topic · Apprise: http://apprise:8000/notify/habits · json: any URL that accepts a POST">
          <input value={f.reminder_url} onChange={e => setF({ ...f, reminder_url: e.target.value })} placeholder="https://ntfy.example.com/habits" />
        </Field>
        <Field label="Format"><Segmented value={f.reminder_format} onChange={v => setF({ ...f, reminder_format: v })} options={[{ value: 'ntfy', label: 'ntfy' }, { value: 'apprise', label: 'Apprise' }, { value: 'json', label: 'JSON webhook' }]} /></Field>
        <div className="row"><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button><button className="btn" onClick={test} disabled={testing || !f.reminder_url}>Send test</button></div>
      </div>
    </Card>
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
    <Card title="Weather">
      <p className="muted">The day's weather from <a href="https://open-meteo.com/" target="_blank" rel="noreferrer">Open-Meteo</a> (free, no key): the condition, high and low, rain, sunshine and daylight, for your home location — or for where your phone was, when the companion app sends its location once a day (<code>POST /api/v1/location</code>). It shows on the month page and the check-in, and Insights compares your scores by weather, temperature and daylight.</p>
      <div className="grid" style={{ gap: 14, maxWidth: 560 }}>
        <Field label="Home location" help={hasHome ? undefined : 'Type a town and pick it from the list.'}>
          {hasHome && (
            <div className={`callout ${unsavedHome ? 'success' : ''} row geo-home`}>
              <span className="geo-pin">⌖</span>
              <span style={{ flex: 1, minWidth: 0 }}><b>{f.weather_place || 'Home'}</b> <span className="muted small">{f.weather_lat.toFixed(3)}, {f.weather_lon.toFixed(3)}</span>{unsavedHome && <span className="muted small"> · not saved yet</span>}</span>
              <button type="button" className="btn ghost sm" onClick={() => setF({ ...f, weather_place: '', weather_lat: 0, weather_lon: 0, weather_enabled: false })}>Change</button>
            </div>
          )}
          {!hasHome && <div className="row"><input value={q} onChange={e => setQ(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); search() } }} placeholder="Town or city" /><button type="button" className="btn" onClick={search} disabled={searching}>{searching ? 'Searching…' : 'Search'}</button></div>}
          {places.length > 0 && (
            <ul className="geo-list">
              {places.map(p => <li key={`${p.lat},${p.lon}`}><button type="button" className="btn ghost" onClick={() => pick(p)}>{p.label} <span className="muted small">{p.lat.toFixed(2)}, {p.lon.toFixed(2)}</span></button></li>)}
            </ul>
          )}
        </Field>
        <Field label="Fetch the weather"><Segmented value={f.weather_enabled ? 'on' : 'off'} onChange={v => setF({ ...f, weather_enabled: v === 'on' })} options={[{ value: 'off', label: 'Off' }, { value: 'on', label: 'On' }]} /></Field>
        <Field label="Temperatures in"><Segmented value={f.weather_units} onChange={v => setF({ ...f, weather_units: v })} options={[{ value: 'celsius', label: '°C' }, { value: 'fahrenheit', label: '°F' }]} /></Field>
        <div className="row"><button className="btn primary" onClick={() => save()} disabled={busy || (f.weather_enabled && !hasHome)}>{dirty ? 'Save' : 'Saved'}</button><button className="btn" onClick={fetchNow} disabled={fetching || !f.weather_enabled}>Fetch now</button>{days !== null && <span className="muted small">{days} days written</span>}{f.weather_last && <span className="muted small">last fetch {f.weather_last.slice(0, 16).replace('T', ' ')}</span>}</div>
        <p className="muted small">Turning it on fetches the last six weeks at once and the rest since your first habit in the background; the scheduler refreshes today and yesterday every three hours. A day's condition can be changed by hand on the month page.</p>
      </div>
    </Card>
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
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
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
    <Card title="API tokens">
      <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. Good for Homepage widgets, iOS Shortcuts (<code>POST /api/v1/checkin</code>), Home Assistant and n8n. See <code>docs/api.md</code>.</p>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage)" /><button className="btn primary">Create</button></form>
      {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? t.last_used_at.slice(0, 16).replace('T', ' ') : 'never'}</td><td className="muted">{t.created_at.slice(0, 10)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Card>
  )
}

function Data() {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const importFile = async (file: File) => {
    setBusy(true)
    try { const r = await api.post<{ created: number; entries: number }>(`${V1}/import`, JSON.parse(await file.text())); toast(`Imported ${r.entries} entries (${r.created} new habits/trackers)`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="two-col">
      <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-habits.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>
      <Card title="Export & import">
        <p className="muted">Portable JSON (re-importable here) or a long CSV for spreadsheets.</p>
        <div className="row mb"><a className="btn" href={`${V1}/export.json`}>Export JSON</a><a className="btn" href={`${V1}/export.csv`}>Export CSV</a></div>
        <label className="btn ghost">{busy ? 'Importing…' : 'Import JSON…'}<input type="file" accept="application/json" hidden onChange={e => e.target.files?.[0] && importFile(e.target.files[0])} /></label>
        <p className="muted small mt">Import merges: habits and trackers are matched by name, entries are upserted.</p>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Habits</b> is a one-page habit tracker modelled on a bullet-journal spread: seven habits in green and red, a sleep score and day rating out of five, a weight row, and one good thing per day. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-habit-tracker" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-habit-tracker</a>. Inspired by the “I Finally Made Habit Tracking Simple” notebook system.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>
            <tr><th>Database</th><td className="mono">{String(data.db_path)} · {(Number(data.db_bytes) / 1024).toFixed(0)} KB</td></tr>
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            <tr><th>Rows</th><td className="mono">{counts.habits} habits · {counts.habit_entries} marks · {counts.trackers} trackers · {counts.tracker_entries} values · {counts.journal} journal lines</td></tr>
          </tbody>
        </table>
      )}
    </Card>
  )
}
