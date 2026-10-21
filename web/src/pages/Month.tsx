import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Day, MonthHabit, MonthPage, MonthTracker, WeatherDay } from '../lib/types'
import { currentMonth, monthTitle, pct, shiftMonth, shortDate } from '../lib/dates'
import { Card, Empty, Loading, Modal, PageHeader, Segmented, useAsync, useToast } from '../components/ui'
import { CONDITIONS, CONDITION_LABEL, WeatherGlyph, weatherLine } from '../components/Weather'
import { Icon } from '../components/icons'
import { useOnChange } from '../components/Layout'
import HabitGrid from '../components/HabitGrid'
import TrackerChart from '../components/TrackerChart'

export default function Month() {
  const { month = currentMonth() } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const { toast } = useToast()
  const { data, setData, loading, error, reload } = useAsync(() => api.get<MonthPage>(`${V1}/months/${month}`), [month])
  useOnChange(() => { reload() })
  const isCurrent = month === currentMonth()
  const [wx, setWx] = useState<{ day: Day; w?: WeatherDay } | null>(null)
  const [imp, setImp] = useState<Day | null>(null)
  const saveImportant = async (date: string, text: string) => {
    try {
      await api.put(`${V1}/journal/${date}`, { important: text })
      setData(await api.get<MonthPage>(`${V1}/months/${month}`))
      setImp(null)
    } catch (e) { toast((e as Error).message, 'err') }
  }

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement)?.tagName === 'INPUT' || (e.target as HTMLElement)?.tagName === 'TEXTAREA') return
      if (e.metaKey || e.ctrlKey || e.altKey || document.querySelector('.modal-backdrop, .palette-backdrop, .menu, .picker-backdrop')) return
      if (e.key === 'ArrowLeft') navigate(`/month/${shiftMonth(month, -1)}`)
      if (e.key === 'ArrowRight' && !isCurrent) navigate(`/month/${shiftMonth(month, 1)}`)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [month, isCurrent, navigate])

  const cycle = useCallback(async (h: MonthHabit, date: string, next: number) => {
    setData(d => d && { ...d, habits: d.habits.map(x => x.id === h.id ? { ...x, entries: { ...x.entries, [date]: next } } : x) })
    try {
      await api.put(`${V1}/habits/${h.id}/entries/${date}`, { status: next })
      const fresh = await api.get<MonthPage>(`${V1}/months/${month}`)
      setData(fresh)
    } catch (e) { toast((e as Error).message, 'err') }
  }, [month, setData, toast])

  const setValue = useCallback(async (t: MonthTracker, date: string, value: number | null) => {
    try {
      await api.put(`${V1}/trackers/${t.id}/entries/${date}`, { value })
      setData(await api.get<MonthPage>(`${V1}/months/${month}`))
    } catch (e) { toast((e as Error).message, 'err') }
  }, [month, setData, toast])

  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return null
  const s = data.summary
  const scored = s.done + s.missed

  return (
    <>
      <PageHeader
        title={<span className="row day-nav"><button type="button" className="btn ghost icon-only" onClick={() => navigate(`/month/${shiftMonth(month, -1)}`)} aria-label="Previous month" title="Previous month (←)"><Icon name="chevron-left" size={18} /></button><span>{monthTitle(month)}</span><button type="button" className="btn ghost icon-only" onClick={() => navigate(`/month/${shiftMonth(month, 1)}`)} disabled={isCurrent} aria-label="Next month" title="Next month (→)"><Icon name="chevron-right" size={18} /></button></span>}
        sub={scored > 0 ? <>More greens than reds is the goal · <b className="ok">{s.done}</b> green, <b className="bad">{s.missed}</b> red, {pct(s.rate)} · {s.perfect_days} perfect day{s.perfect_days === 1 ? '' : 's'} · {s.journal_days} journal line{s.journal_days === 1 ? '' : 's'}</> : 'Click a cell to mark a habit: once for done, twice for missed.'}
        actions={<>{!isCurrent && <Link className="btn" to="/month"><Icon name="undo" size={15} />This month</Link>}<Link className="btn primary" to="/today"><Icon name="check" size={15} strokeWidth={2.4} />Check in today</Link></>}
      />

      <Card flush className="mb">
        {data.habits.length === 0 && <div className="pad"><Empty title="No habits on this page yet">Pick up to seven things you want to do (or avoid) this month. <Link to="/manage?new=habit">Add habits →</Link></Empty></div>}
        {/* one scroller for the grid and the charts, so the days stay in line on a phone */}
        <div className="grid-scroll">
          <div className="month-sheet">
            {data.habits.length > 0 && <HabitGrid days={data.days} habits={data.habits} onCycle={cycle} onEdit={h => navigate(`/manage?habit=${h.id}`)} weather={settings?.weather_enabled ? data.weather : null} units={settings?.weather_units} onWeather={(day, w) => setWx({ day, w })} important={data.important} onDay={setImp} />}
            {data.trackers.length > 0 && (
              <div className="trackers">
                {data.trackers.map(t => <TrackerChart key={t.id} days={data.days} tracker={t} onSet={setValue} />)}
              </div>
            )}
          </div>
        </div>
      </Card>

      <ImportantDays page={data} onOpen={setImp} />

      <Journal page={data} prompt={settings?.journal_prompt || 'What was great about today?'} badPrompt={settings?.bad_thing_enabled ? (settings.bad_prompt || 'And one thing that was bad?') : null}
        onSaved={(date, text) => setData(d => d && { ...d, journal: { ...d.journal, [date]: text }, summary: { ...d.summary, journal_days: Object.values({ ...d.journal, [date]: text }).filter(Boolean).length } })}
        onSavedBad={(date, text) => setData(d => d && { ...d, journal_bad: { ...d.journal_bad, [date]: text } })} />

      {imp && <ImportantModal day={imp} text={data.important?.[imp.date] || ''} onSave={t => saveImportant(imp.date, t)} onClose={() => setImp(null)} />}

      {wx && (
        <Modal title={`Weather on ${shortDate(wx.day.date)}`} onClose={() => setWx(null)}>
          {wx.w ? <p className="muted">{weatherLine(wx.w, settings?.weather_units)}{wx.w.overridden ? ' · set by hand' : ''}</p> : <p className="muted">Nothing fetched for this day yet.</p>}
          <p className="small muted">Set the condition by hand when you were somewhere else or the station was wrong. The numbers stay as fetched.</p>
          <div className="row wrap wx-choices">
            {CONDITIONS.map(c => (
              <button type="button" key={c} className={`btn ${wx.w?.condition === c ? 'primary' : ''}`} onClick={async () => {
                try { await api.put(`${V1}/weather/${wx.day.date}`, { condition: c }); setData(await api.get<MonthPage>(`${V1}/months/${month}`)); setWx(null) } catch (e) { toast((e as Error).message, 'err') }
              }}><WeatherGlyph condition={c} size={15} />{CONDITION_LABEL[c]}</button>
            ))}
            {wx.w?.overridden && <button type="button" className="btn ghost" onClick={async () => {
              try { await api.put(`${V1}/weather/${wx.day.date}`, { condition: '' }); setData(await api.get<MonthPage>(`${V1}/months/${month}`)); setWx(null) } catch (e) { toast((e as Error).message, 'err') }
            }}>Back to the fetched one</button>}
          </div>
        </Modal>
      )}
    </>
  )
}

// The month's important days, red, oldest first; a click opens the note.
function ImportantDays({ page, onOpen }: { page: MonthPage; onOpen: (d: Day) => void }) {
  const days = page.days.filter(d => page.important?.[d.date])
  if (days.length === 0) return null
  return (
    <Card title={<span className="important-prompt">Important days</span>} actions={<span className="muted small">{days.length} this month · click a day's number above to add one</span>} className="mb">
      <ul className="important-list">
        {days.map(d => (
          <li key={d.date}>
            <button type="button" onClick={() => onOpen(d)}><span className="jdate">{shortDate(d.date)}</span><span className="itext">{page.important?.[d.date]}</span></button>
          </li>
        ))}
      </ul>
    </Card>
  )
}

function ImportantModal({ day, text, onSave, onClose }: { day: Day; text: string; onSave: (t: string) => Promise<void>; onClose: () => void }) {
  const [v, setV] = useState(text)
  const [saving, setSaving] = useState(false)
  const save = async (t: string) => { setSaving(true); await onSave(t.trim()); setSaving(false) }
  return (
    <Modal title={`Important · ${shortDate(day.date)}`} onClose={onClose}>
      <p className="small muted">Something to remember about this day. The day turns red on the month page.</p>
      <textarea rows={4} autoFocus value={v} onChange={e => setV(e.target.value)} maxLength={4000} placeholder="What happened, or what is coming…" style={{ width: '100%' }}
        onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save(v) }} />
      <div className="form-actions mt-s">
        {text && <button type="button" className="btn ghost danger-text" disabled={saving} onClick={() => save('')}>Remove</button>}
        <span className="spacer" />
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="button" className="btn primary" disabled={saving || v.trim() === text} onClick={() => save(v)}>{saving ? 'Saving…' : 'Save'}</button>
      </div>
    </Modal>
  )
}

function Journal({ page, prompt, badPrompt, onSaved, onSavedBad }: { page: MonthPage; prompt: string; badPrompt: string | null; onSaved: (date: string, text: string) => void; onSavedBad: (date: string, text: string) => void }) {
  const days = page.days.filter(d => !d.future)
  return (
    <Card title={badPrompt ? <span className="row wrap"><span>{prompt}</span><span className="muted">·</span><span className="bad-prompt">{badPrompt}</span></span> : prompt} actions={<span className="muted small">{page.summary.journal_days} of {days.length} days</span>}>
      <div className={`journal ${badPrompt ? 'two' : ''}`}>
        {days.slice().reverse().map(d => <JournalLine key={d.date} date={d.date} today={d.today} text={page.journal[d.date] || ''} bad={badPrompt ? (page.journal_bad[d.date] || '') : null} onSaved={onSaved} onSavedBad={onSavedBad} />)}
      </div>
    </Card>
  )
}

function JournalLine({ date, text, bad, today, onSaved, onSavedBad }: { date: string; text: string; bad: string | null; today: boolean; onSaved: (date: string, text: string) => void; onSavedBad: (date: string, text: string) => void }) {
  return (
    <div className={`journal-line ${today ? 'today' : ''} ${text || bad ? 'filled' : ''}`}>
      <span className="jdate">{shortDate(date)}</span>
      <JournalInput date={date} field="text" value={text} placeholder={today ? 'One good thing from today…' : ''} onSaved={onSaved} />
      {bad !== null && <JournalInput date={date} field="bad" value={bad} placeholder={today ? 'One bad thing…' : ''} onSaved={onSavedBad} />}
    </div>
  )
}

function JournalInput({ date, field, value, placeholder, onSaved }: { date: string; field: 'text' | 'bad'; value: string; placeholder: string; onSaved: (date: string, text: string) => void }) {
  const [v, setV] = useState(value)
  const [saving, setSaving] = useState(false)
  const last = useRef(value)
  const { toast } = useToast()
  useEffect(() => { setV(value); last.current = value }, [value])
  const save = async () => {
    if (v === last.current) return
    setSaving(true)
    try { await api.put(`${V1}/journal/${date}`, { [field]: v }); last.current = v; onSaved(date, v) } catch (e) { toast((e as Error).message, 'err') } finally { setSaving(false) }
  }
  return (
    <span className={`jfield ${field}`}>
      <input value={v} onChange={e => setV(e.target.value)} onBlur={save} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} placeholder={placeholder} maxLength={4000} />
      {saving && <span className="spinner sm" />}
    </span>
  )
}
