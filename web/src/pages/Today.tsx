import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { TodayHabit, TodayPage, TodayTracker } from '../lib/types'
import { fmtNum, longDate, shiftDate, todayISO } from '../lib/dates'
import { Card, Loading, PageHeader, Segmented, bandFor, useAsync, useToast } from '../components/ui'

export default function Today() {
  const { date = todayISO() } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const { toast } = useToast()
  const { data, setData, loading, error } = useAsync(() => api.get<TodayPage>(`${V1}/days/${date}`), [date])
  const isToday = date === todayISO()

  const setHabit = async (h: TodayHabit, status: number) => {
    const next = h.status === status ? 0 : status
    setData(d => d && { ...d, habits: d.habits.map(x => x.id === h.id ? { ...x, status: next } : x) })
    try { await api.put(`${V1}/habits/${h.id}/entries/${date}`, { status: next }); setData(await api.get<TodayPage>(`${V1}/days/${date}`)) } catch (e) { toast((e as Error).message, 'err') }
  }
  const setTracker = async (t: TodayTracker, value: number | null) => {
    setData(d => d && { ...d, trackers: d.trackers.map(x => x.id === t.id ? { ...x, value } : x) })
    try { await api.put(`${V1}/trackers/${t.id}/entries/${date}`, { value }) } catch (e) { toast((e as Error).message, 'err') }
  }

  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const scheduled = data.habits.filter(h => h.scheduled)
  const unscheduled = data.habits.filter(h => !h.scheduled)
  const marked = scheduled.filter(h => h.status !== 0).length
  const allMarked = scheduled.length > 0 && marked === scheduled.length
  const done = scheduled.filter(h => h.status === 1).length

  return (
    <div className="today">
      <PageHeader
        title={<span className="row"><button className="btn ghost icon" onClick={() => navigate(`/today/${shiftDate(date, -1)}`)} aria-label="Previous day">‹</button><span>{isToday ? 'Today' : longDate(date)}</span><button className="btn ghost icon" onClick={() => navigate(`/today/${shiftDate(date, 1)}`)} disabled={isToday} aria-label="Next day">›</button></span>}
        sub={isToday ? longDate(date) : undefined}
        actions={<>{!isToday && <Link className="btn" to="/today">Back to today</Link>}<Link className="btn" to={`/month/${date.slice(0, 7)}`}>Month page</Link></>}
      />

      <div className="progress-row">
        <div className="ring" style={{ ['--p' as string]: scheduled.length ? done / scheduled.length : 0 }}><span>{done}<small>/{scheduled.length}</small></span></div>
        <div>
          <div className="big">{allMarked ? (done === scheduled.length ? 'Perfect day.' : `${done} green, ${scheduled.length - done} red.`) : `${scheduled.length - marked} left to mark`}</div>
          <div className="muted small">Takes a minute. Green = did it, red = didn't. The aim is more greens than reds.</div>
        </div>
      </div>

      <Card title={<>Habits <span className="count">{done}/{scheduled.length}</span></>} flush className="section">
        {scheduled.length === 0 && unscheduled.length === 0 && <div className="pad muted">No habits yet. <Link to="/manage">Add some →</Link></div>}
        <ul className="checkin-list">
          {scheduled.map(h => <HabitRow key={h.id} h={h} onSet={setHabit} />)}
          {unscheduled.length > 0 && <li className="divider muted small">Not scheduled today</li>}
          {unscheduled.map(h => <HabitRow key={h.id} h={h} onSet={setHabit} dim />)}
        </ul>
      </Card>

      {data.trackers.length > 0 && (
        <Card title={<>Scores & measurements <span className="count">{data.trackers.filter(t => t.value !== null).length}/{data.trackers.length}</span></>} flush className="section">
          <ul className="checkin-list">
            {data.trackers.map(t => <TrackerRow key={t.id} t={t} onSet={setTracker} />)}
          </ul>
        </Card>
      )}

      <Card title={settings?.journal_prompt || 'What was great about today?'} flush className="section">
        <JournalBox date={date} field="text" text={data.journal} onSaved={t => setData(d => d && { ...d, journal: t })} placeholder="One thing that happened today that was great." />
      </Card>

      {settings?.bad_thing_enabled && (
        <Card title={settings.bad_prompt || 'And one thing that was bad?'} flush className="section bad">
          <JournalBox date={date} field="bad" text={data.journal_bad} onSaved={t => setData(d => d && { ...d, journal_bad: t })} placeholder="One thing that was bad — name it and let it go." />
        </Card>
      )}
    </div>
  )
}

function HabitRow({ h, onSet, dim }: { h: TodayHabit; onSet: (h: TodayHabit, s: number) => void; dim?: boolean }) {
  return (
    <li className={`checkin ${dim ? 'dim' : ''} ${h.status === 1 ? 'is-done' : h.status === -1 ? 'is-missed' : ''}`}>
      <div className="ci-name">
        {h.color && <span className="swatch" style={{ background: h.color }} />}
        <span>{h.name}</span>
        {h.streak > 1 && <span className="streak">🔥{h.streak}</span>}
      </div>
      <div className={`ci-actions ${h.status !== 0 ? 'has-choice' : ''}`}>
        <button className={`mark done ${h.status === 1 ? 'active' : ''}`} onClick={() => onSet(h, 1)} aria-pressed={h.status === 1} aria-label={`${h.name} done`}><span>✓</span></button>
        <button className={`mark missed ${h.status === -1 ? 'active' : ''}`} onClick={() => onSet(h, -1)} aria-pressed={h.status === -1} aria-label={`${h.name} missed`}><span>✗</span></button>
      </div>
    </li>
  )
}

function TrackerRow({ t, onSet }: { t: TodayTracker; onSet: (t: TodayTracker, v: number | null) => void }) {
  const [draft, setDraft] = useState(t.value !== null ? String(t.value) : '')
  useEffect(() => { setDraft(t.value !== null ? String(t.value) : '') }, [t.value])
  if (t.kind === 'yesno') {
    // "yes" is good only when higher is better (e.g. "Slept well?"); for "Headache?" it is bad.
    const yesTone = t.higher_is_better ? 'good' : 'bad', noTone = t.higher_is_better ? 'bad' : 'good'
    return (
      <li className={`checkin yesno-row ${t.value === 1 ? `is-${yesTone}` : t.value === 0 ? `is-${noTone}` : ''}`}>
        <div className="ci-name"><span>{t.name}</span>{t.previous !== null && <span className="muted small">yesterday {t.previous === 1 ? 'yes' : 'no'}</span>}</div>
        <div className={`ci-actions ${t.value !== null ? 'has-choice' : ''}`}>
          <button className={`mark yn ${yesTone} ${t.value === 1 ? 'active' : ''}`} onClick={() => onSet(t, t.value === 1 ? null : 1)} aria-pressed={t.value === 1}>Yes</button>
          <button className={`mark yn ${noTone} ${t.value === 0 ? 'active' : ''}`} onClick={() => onSet(t, t.value === 0 ? null : 0)} aria-pressed={t.value === 0}>No</button>
        </div>
      </li>
    )
  }
  if (t.kind === 'scale') {
    const steps: number[] = []
    for (let v = t.min_value; v <= t.max_value + 1e-9; v += t.step) steps.push(Number(v.toFixed(4)))
    return (
      <li className={`checkin scale-row ${t.value !== null ? 'has-value' : ''}`}>
        <div className="ci-name"><span>{t.name}</span><span className="muted small">{t.value !== null ? `${fmtNum(t.value)} of ${fmtNum(t.max_value)}` : `${fmtNum(t.min_value)}–${fmtNum(t.max_value)}`}{!t.higher_is_better ? ' · lower is better' : ''}</span></div>
        <div className="scale" role="radiogroup" aria-label={t.name}>
          {steps.map(s => <button key={s} role="radio" aria-checked={t.value === s} className={`pill ${bandFor(s, t.min_value, t.max_value, t.higher_is_better)} ${t.value === s ? 'active' : ''}`} onClick={() => onSet(t, t.value === s ? null : s)}>{fmtNum(s)}</button>)}
        </div>
      </li>
    )
  }
  const commit = () => {
    const v = draft.trim() === '' ? null : Number(draft.replace(',', '.'))
    if (v !== null && Number.isNaN(v)) return
    if (v !== t.value) onSet(t, v)
  }
  const delta = t.value !== null && t.previous !== null ? t.value - t.previous : null
  return (
    <li className="checkin">
      <div className="ci-name"><span>{t.name}</span>{t.previous !== null && <span className="muted small">last {fmtNum(t.previous)} {t.unit}</span>}</div>
      <div className="row">
        {delta !== null && delta !== 0 && <span className={`delta ${(delta < 0) === !t.higher_is_better ? 'ok' : 'bad'}`}>{delta > 0 ? '+' : ''}{fmtNum(delta)}</span>}
        <label className="num-field">
          <input inputMode="decimal" value={draft} onChange={e => setDraft(e.target.value)} onBlur={commit} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} placeholder="–" />
          {t.unit && <span className="unit">{t.unit}</span>}
        </label>
      </div>
    </li>
  )
}

function JournalBox({ date, text, field, placeholder, onSaved }: { date: string; text: string; field: 'text' | 'bad'; placeholder: string; onSaved: (t: string) => void }) {
  const [v, setV] = useState(text)
  const last = useRef(text)
  const [state, setState] = useState<'idle' | 'saving' | 'saved'>('idle')
  const { toast } = useToast()
  useEffect(() => { setV(text); last.current = text }, [text, date])
  const save = async () => {
    if (v === last.current) return
    setState('saving')
    try { await api.put(`${V1}/journal/${date}`, { [field]: v }); last.current = v; onSaved(v); setState('saved'); setTimeout(() => setState('idle'), 1500) } catch (e) { toast((e as Error).message, 'err'); setState('idle') }
  }
  return (
    <div className="journal-box">
      <textarea rows={3} value={v} onChange={e => setV(e.target.value)} onBlur={save} placeholder={placeholder} maxLength={4000} />
      <div className="journal-foot">
        <span className="muted small">{state === 'saving' ? 'Saving…' : state === 'saved' ? '✓ Saved' : v === last.current && v ? '✓ Saved' : 'Saves when you click away'}</span>
        <button className="btn sm primary" onClick={save} disabled={v === last.current}>Save</button>
      </div>
    </div>
  )
}

export function Landing() {
  const { settings } = useApp()
  const navigate = useNavigate()
  useEffect(() => { navigate(settings?.landing === 'today' ? '/today' : '/month', { replace: true }) }, [settings?.landing, navigate])
  return <Loading />
}

export { Segmented }
