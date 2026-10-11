import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Habit, Tracker } from '../lib/types'
import { WEEKDAY_SHORT, currentMonth, monthTitle, shiftMonth, todayISO } from '../lib/dates'
import { Card, Confirm, Empty, Field, Loading, Modal, PageHeader, Segmented, useAsync, useToast } from '../components/ui'

const COLORS = ['', '#2563eb', '#7c3aed', '#0891b2', '#059669', '#d97706', '#dc2626', '#db2777', '#4b5563']

export default function Manage() {
  const [params, setParams] = useSearchParams()
  const { toast } = useToast()
  const habits = useAsync(() => api.get<Habit[]>(`${V1}/habits?all=1`), [])
  const trackers = useAsync(() => api.get<Tracker[]>(`${V1}/trackers?all=1`), [])
  const [editHabit, setEditHabit] = useState<Partial<Habit> | null>(null)
  const [editTracker, setEditTracker] = useState<Partial<Tracker> | null>(null)
  const [confirm, setConfirm] = useState<{ title: string; message: string; run: () => Promise<void> } | null>(null)

  useEffect(() => {
    const id = Number(params.get('habit'))
    if (id && habits.data) { const h = habits.data.find(x => x.id === id); if (h) { setEditHabit(h); params.delete('habit'); setParams(params, { replace: true }) } }
  }, [habits.data]) // eslint-disable-line react-hooks/exhaustive-deps

  const today = todayISO()
  const active = (x: { end_date: string | null }) => !x.end_date || x.end_date >= today
  const run = async (fn: () => Promise<unknown>, msg?: string) => { try { await fn(); await Promise.all([habits.reload(), trackers.reload()]); if (msg) toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  const move = (list: { id: number }[], id: number, dir: -1 | 1, url: string) => {
    const ids = list.map(x => x.id); const i = ids.indexOf(id); const j = i + dir
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    run(() => api.put(url, { ids }))
  }

  if (!habits.data || !trackers.data) return <Loading />
  const activeHabits = habits.data.filter(active), endedHabits = habits.data.filter(x => !active(x))
  const activeTrackers = trackers.data.filter(active), endedTrackers = trackers.data.filter(x => !active(x))

  return (
    <>
      <PageHeader title="Habits & trackers" sub="Swap habits out each month if they stop being relevant. Seven is plenty." actions={<><button className="btn" onClick={() => setEditTracker({ kind: 'scale', min_value: 1, max_value: 5, step: 1, higher_is_better: true })}>+ Tracker</button><button className="btn primary" onClick={() => setEditHabit({ days: '1234567' })}>+ Habit</button></>} />

      <div className="two-col">
        <Card title={`Habits (${activeHabits.length})`} flush>
          {activeHabits.length === 0 && <div className="pad"><Empty title="No active habits">Things like Run, Workout, Stretch, Morning page, No alcohol.</Empty></div>}
          <ul className="manage-list">
            {activeHabits.map((h, i) => (
              <li key={h.id}>
                <div className="order"><button className="btn ghost sm" disabled={i === 0} onClick={() => move(activeHabits, h.id, -1, `${V1}/habits/reorder`)} aria-label="Move up">↑</button><button className="btn ghost sm" disabled={i === activeHabits.length - 1} onClick={() => move(activeHabits, h.id, 1, `${V1}/habits/reorder`)} aria-label="Move down">↓</button></div>
                <button className="item" onClick={() => setEditHabit(h)}>
                  {h.color && <span className="swatch" style={{ background: h.color }} />}
                  <span className="nm">{h.name}</span>
                  <span className="muted small">{daysLabel(h.days)} · since {h.start_date}{h.end_date ? ` · until ${h.end_date}` : ''}</span>
                </button>
                <div className="row">
                  <button className="btn ghost sm" title="Take it off next month's page" onClick={() => setConfirm({ title: `Stop tracking “${h.name}”?`, message: `It stays on ${monthTitle(currentMonth())}'s page and disappears from ${monthTitle(shiftMonth(currentMonth(), 1))} onwards. History is kept.`, run: () => run(() => api.post(`${V1}/habits/${h.id}/end`, {}), 'Habit ends this month') })}>End</button>
                  <button className="btn ghost sm danger-text" title="Delete the habit and all its marks" onClick={() => setConfirm({ title: `Delete “${h.name}”?`, message: 'This also deletes every day you marked for it. To keep the history, use End instead.', run: () => run(() => api.del(`${V1}/habits/${h.id}`), 'Habit deleted') })}>Delete</button>
                </div>
              </li>
            ))}
          </ul>
          {endedHabits.length > 0 && <>
            <div className="list-section muted small">Ended</div>
            <ul className="manage-list ended">
              {endedHabits.map(h => (
                <li key={h.id}>
                  <button className="item" onClick={() => setEditHabit(h)}><span className="nm">{h.name}</span><span className="muted small">{h.start_date} → {h.end_date}</span></button>
                  <div className="row"><button className="btn ghost sm" onClick={() => run(() => api.post(`${V1}/habits/${h.id}/resume`), 'Habit resumed')}>Resume</button><button className="btn ghost sm danger-text" onClick={() => setConfirm({ title: `Delete “${h.name}”?`, message: 'This also deletes every day you marked for it. To keep the history, leave it ended instead.', run: () => run(() => api.del(`${V1}/habits/${h.id}`), 'Habit deleted') })}>Delete</button></div>
                </li>
              ))}
            </ul>
          </>}
        </Card>

        <Card title={`Trackers (${activeTrackers.length})`} flush>
          {activeTrackers.length === 0 && <div className="pad"><Empty title="No trackers">A 1–5 sleep score and day rating, plus weight, is the classic set.</Empty></div>}
          <ul className="manage-list">
            {activeTrackers.map((t, i) => (
              <li key={t.id}>
                <div className="order"><button className="btn ghost sm" disabled={i === 0} onClick={() => move(activeTrackers, t.id, -1, `${V1}/trackers/reorder`)} aria-label="Move up">↑</button><button className="btn ghost sm" disabled={i === activeTrackers.length - 1} onClick={() => move(activeTrackers, t.id, 1, `${V1}/trackers/reorder`)} aria-label="Move down">↓</button></div>
                <button className="item" onClick={() => setEditTracker(t)}>
                  <span className="nm">{t.name}</span>
                  <span className="muted small">{t.kind === 'scale' ? `scale ${t.min_value}–${t.max_value}` : t.kind === 'yesno' ? 'yes / no' : `number${t.unit ? ' · ' + t.unit : ''}`}{t.kind === 'yesno' ? (t.higher_is_better ? ' · yes is good' : ' · yes is bad') : t.higher_is_better ? '' : ' · lower is better'}</span>
                </button>
                <div className="row">
                  <button className="btn ghost sm" title="Take it off next month's page" onClick={() => setConfirm({ title: `Stop tracking “${t.name}”?`, message: 'It stays on this month’s page and disappears from next month. History is kept.', run: () => run(() => api.post(`${V1}/trackers/${t.id}/end`, {}), 'Tracker ends this month') })}>End</button>
                  <button className="btn ghost sm danger-text" title="Delete the tracker and all its values" onClick={() => setConfirm({ title: `Delete “${t.name}”?`, message: 'This also deletes every value recorded for it. To keep the history, use End instead.', run: () => run(() => api.del(`${V1}/trackers/${t.id}`), 'Tracker deleted') })}>Delete</button>
                </div>
              </li>
            ))}
          </ul>
          {endedTrackers.length > 0 && <>
            <div className="list-section muted small">Ended</div>
            <ul className="manage-list ended">
              {endedTrackers.map(t => (
                <li key={t.id}>
                  <button className="item" onClick={() => setEditTracker(t)}><span className="nm">{t.name}</span><span className="muted small">{t.start_date} → {t.end_date}</span></button>
                  <div className="row"><button className="btn ghost sm" onClick={() => run(() => api.post(`${V1}/trackers/${t.id}/resume`), 'Tracker resumed')}>Resume</button><button className="btn ghost sm danger-text" onClick={() => setConfirm({ title: `Delete “${t.name}”?`, message: 'This also deletes every value recorded for it.', run: () => run(() => api.del(`${V1}/trackers/${t.id}`), 'Tracker deleted') })}>Delete</button></div>
                </li>
              ))}
            </ul>
          </>}
        </Card>
      </div>

      {editHabit && <HabitModal habit={editHabit} onClose={() => setEditHabit(null)} onSaved={() => { setEditHabit(null); habits.reload() }} onDelete={h => { setEditHabit(null); setConfirm({ title: `Delete “${h.name}”?`, message: 'This also deletes every day you marked for it. To keep the history, end it instead.', run: () => run(() => api.del(`${V1}/habits/${h.id}`), 'Habit deleted') }) }} />}
      {editTracker && <TrackerModal tracker={editTracker} onClose={() => setEditTracker(null)} onSaved={() => { setEditTracker(null); trackers.reload() }} onDelete={t => { setEditTracker(null); setConfirm({ title: `Delete “${t.name}”?`, message: 'This also deletes every value recorded for it. To keep the history, use End instead.', run: () => run(() => api.del(`${V1}/trackers/${t.id}`), 'Tracker deleted') }) }} />}
      {confirm && <Confirm title={confirm.title} message={confirm.message} onCancel={() => setConfirm(null)} onConfirm={async () => { const c = confirm; setConfirm(null); await c.run() }} />}
    </>
  )
}

function daysLabel(days: string): string {
  if (days === '1234567') return 'every day'
  if (days === '12345') return 'weekdays'
  if (days === '67') return 'weekends'
  return days.split('').map(d => WEEKDAY_SHORT[Number(d) - 1]).join(' ')
}

function HabitModal({ habit, onClose, onSaved, onDelete }: { habit: Partial<Habit>; onClose: () => void; onSaved: () => void; onDelete: (h: Habit) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: habit.name || '', color: habit.color || '', days: habit.days || '1234567', start_date: habit.start_date || todayISO(), end_date: habit.end_date || '' })
  const [busy, setBusy] = useState(false)
  const toggleDay = (d: number) => setF(x => { const has = x.days.includes(String(d)); const next = has ? x.days.replace(String(d), '') : (x.days + d).split('').sort().join(''); return { ...x, days: next || String(d) } })
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const body = { ...f, end_date: f.end_date || null }
      if (habit.id) await api.put(`${V1}/habits/${habit.id}`, body); else await api.post(`${V1}/habits`, body)
      toast(habit.id ? 'Habit saved' : 'Habit added'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={habit.id ? 'Edit habit' : 'New habit'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required autoFocus placeholder="Run, Stretch, No alcohol…" maxLength={60} /></Field>
        <Field label="Scheduled on" help="Unscheduled days are greyed out and never count as missed.">
          <div className="row wrap">
            {WEEKDAY_SHORT.map((w, i) => <button type="button" key={w} className={`chip ${f.days.includes(String(i + 1)) ? 'active' : ''}`} onClick={() => toggleDay(i + 1)}>{w}</button>)}
            <button type="button" className="btn ghost sm" onClick={() => setF({ ...f, days: '1234567' })}>Every day</button>
            <button type="button" className="btn ghost sm" onClick={() => setF({ ...f, days: '12345' })}>Weekdays</button>
          </div>
        </Field>
        <Field label="Colour (optional)">
          <div className="row wrap">{COLORS.map(c => <button type="button" key={c || 'none'} className={`color-dot ${f.color === c ? 'active' : ''}`} style={{ background: c || 'transparent' }} onClick={() => setF({ ...f, color: c })} aria-label={c || 'no colour'}>{c ? '' : '∅'}</button>)}</div>
        </Field>
        <div className="two-col tight">
          <Field label="Start date" help="First day it appears on a page."><input type="date" value={f.start_date} onChange={e => setF({ ...f, start_date: e.target.value })} required /></Field>
          <Field label="End date (optional)" help="Last day it appears."><input type="date" value={f.end_date} onChange={e => setF({ ...f, end_date: e.target.value })} /></Field>
        </div>
        <div className="form-actions">
          {habit.id && <button type="button" className="btn ghost danger-text" onClick={() => onDelete(habit as Habit)}>Delete</button>}
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy}>{habit.id ? 'Save' : 'Add habit'}</button>
        </div>
      </form>
    </Modal>
  )
}

function TrackerModal({ tracker, onClose, onSaved, onDelete }: { tracker: Partial<Tracker>; onClose: () => void; onSaved: () => void; onDelete: (t: Tracker) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: tracker.name || '', kind: tracker.kind || 'scale', min_value: tracker.min_value ?? 1, max_value: tracker.max_value ?? 5, step: tracker.step ?? 1, unit: tracker.unit || '', higher_is_better: tracker.higher_is_better ?? true, start_date: tracker.start_date || todayISO(), end_date: tracker.end_date || '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const body = { ...f, min_value: Number(f.min_value), max_value: Number(f.max_value), step: Number(f.step), end_date: f.end_date || null }
      if (tracker.id) await api.put(`${V1}/trackers/${tracker.id}`, body); else await api.post(`${V1}/trackers`, body)
      toast(tracker.id ? 'Tracker saved' : 'Tracker added'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={tracker.id ? 'Edit tracker' : 'New tracker'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required autoFocus placeholder="Sleep score, Day rating, Weight, Steps…" maxLength={60} /></Field>
        <Field label="Type">
          <Segmented value={f.kind} onChange={k => setF(k === 'number' ? { ...f, kind: k, min_value: 0, max_value: 0, step: 0.1, higher_is_better: false } : k === 'yesno' ? { ...f, kind: k, min_value: 0, max_value: 1, step: 1, unit: '', higher_is_better: false } : { ...f, kind: k, min_value: 1, max_value: 5, step: 1, higher_is_better: true })} options={[{ value: 'scale', label: 'Score (1–5)' }, { value: 'number', label: 'Measurement' }, { value: 'yesno', label: 'Yes / no' }]} />
        </Field>
        {f.kind === 'scale' ? (
          <div className="three-col tight">
            <Field label="Min"><input type="number" step="any" value={f.min_value} onChange={e => setF({ ...f, min_value: Number(e.target.value) })} /></Field>
            <Field label="Max"><input type="number" step="any" value={f.max_value} onChange={e => setF({ ...f, max_value: Number(e.target.value) })} /></Field>
            <Field label="Step"><input type="number" step="any" min={0.1} value={f.step} onChange={e => setF({ ...f, step: Number(e.target.value) })} /></Field>
          </div>
        ) : f.kind === 'yesno' ? (
          <p className="muted small">A question you answer with yes or no each day — “Headache?”, “Slept well?”, “Rained?”. Not a habit: nothing is “missed”, it is just recorded. Insights compares your scores on yes-days and no-days.</p>
        ) : (
          <div className="two-col tight">
            <Field label="Unit"><input value={f.unit} onChange={e => setF({ ...f, unit: e.target.value })} placeholder="kg, steps, h, km" maxLength={12} /></Field>
            <Field label="Step"><input type="number" step="any" min={0.001} value={f.step} onChange={e => setF({ ...f, step: Number(e.target.value) })} /></Field>
          </div>
        )}
        <Field label="Direction" help={f.kind === 'yesno' ? 'Is “yes” the good answer? For “Headache?” it is not; for “Slept well?” it is.' : 'Decides which end gets the green band and whether a drop counts as good.'}>
          <Segmented value={f.higher_is_better ? 'up' : 'down'} onChange={v => setF({ ...f, higher_is_better: v === 'up' })} options={f.kind === 'yesno' ? [{ value: 'up', label: 'Yes is good' }, { value: 'down', label: 'Yes is bad' }] : [{ value: 'up', label: 'Higher is better' }, { value: 'down', label: 'Lower is better' }]} />
        </Field>
        <div className="two-col tight">
          <Field label="Start date"><input type="date" value={f.start_date} onChange={e => setF({ ...f, start_date: e.target.value })} required /></Field>
          <Field label="End date (optional)"><input type="date" value={f.end_date} onChange={e => setF({ ...f, end_date: e.target.value })} /></Field>
        </div>
        <div className="form-actions">
          {tracker.id && <button type="button" className="btn ghost danger-text" onClick={() => onDelete(tracker as Tracker)}>Delete</button>}
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy}>{tracker.id ? 'Save' : 'Add tracker'}</button>
        </div>
      </form>
    </Modal>
  )
}
