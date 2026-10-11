import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { MonthHabit, MonthPage, MonthTracker } from '../lib/types'
import { currentMonth, monthTitle, pct, shiftMonth, shortDate } from '../lib/dates'
import { Card, Empty, Loading, PageHeader, useAsync, useToast } from '../components/ui'
import HabitGrid from '../components/HabitGrid'
import TrackerChart from '../components/TrackerChart'

export default function Month() {
  const { month = currentMonth() } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const { toast } = useToast()
  const { data, setData, loading, error } = useAsync(() => api.get<MonthPage>(`${V1}/months/${month}`), [month])
  const isCurrent = month === currentMonth()

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement)?.tagName === 'INPUT' || (e.target as HTMLElement)?.tagName === 'TEXTAREA') return
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
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const s = data.summary
  const scored = s.done + s.missed

  return (
    <>
      <PageHeader
        title={<span className="row"><button className="btn ghost icon" onClick={() => navigate(`/month/${shiftMonth(month, -1)}`)} aria-label="Previous month">‹</button><span>{monthTitle(month)}</span><button className="btn ghost icon" onClick={() => navigate(`/month/${shiftMonth(month, 1)}`)} disabled={isCurrent} aria-label="Next month">›</button></span>}
        sub={scored > 0 ? <>More greens than reds is the goal · <b className="ok">{s.done}</b> green, <b className="bad">{s.missed}</b> red, {pct(s.rate)} · {s.perfect_days} perfect day{s.perfect_days === 1 ? '' : 's'} · {s.journal_days} journal line{s.journal_days === 1 ? '' : 's'}</> : 'Click a cell to mark a habit: once for done, twice for missed.'}
        actions={<>{!isCurrent && <Link className="btn" to="/month">This month</Link>}<Link className="btn primary" to="/today">Check in today</Link></>}
      />

      <Card flush className="mb">
        {data.habits.length === 0
          ? <div className="pad"><Empty title="No habits on this page yet">Pick up to seven things you want to do (or avoid) this month. <Link to="/manage">Add habits →</Link></Empty></div>
          : <HabitGrid days={data.days} habits={data.habits} onCycle={cycle} onEdit={h => navigate(`/manage?habit=${h.id}`)} />}
        {data.trackers.length > 0 && (
          <div className="trackers">
            {data.trackers.map(t => <TrackerChart key={t.id} days={data.days} tracker={t} onSet={setValue} />)}
          </div>
        )}
      </Card>

      <Journal page={data} prompt={settings?.journal_prompt || 'What was great about today?'} badPrompt={settings?.bad_thing_enabled ? (settings.bad_prompt || 'And one thing that was bad?') : null}
        onSaved={(date, text) => setData(d => d && { ...d, journal: { ...d.journal, [date]: text }, summary: { ...d.summary, journal_days: Object.values({ ...d.journal, [date]: text }).filter(Boolean).length } })}
        onSavedBad={(date, text) => setData(d => d && { ...d, journal_bad: { ...d.journal_bad, [date]: text } })} />
    </>
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
