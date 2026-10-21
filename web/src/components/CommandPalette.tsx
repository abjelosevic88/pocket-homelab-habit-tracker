// ⌘K / "/" — jump to a habit, a tracker, a month or a day, or run a command from the keyboard.
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { currentMonth, longDate, monthTitle, shiftDate, shiftMonth, todayISO } from '../lib/dates'
import type { Habit, Tracker } from '../lib/types'
import { Icon } from './icons'

interface Command { id: string; label: string; icon: string; hint?: string; keywords?: string; run: () => void }

export function CommandPalette({ onClose, onShortcuts }: { onClose: () => void; onShortcuts: () => void }) {
  const { theme, setThemeMode } = useApp()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const [habits, setHabits] = useState<Habit[]>([])
  const [trackers, setTrackers] = useState<Tracker[]>([])
  const list = useRef<HTMLDivElement>(null)
  useEffect(() => {
    api.get<Habit[]>(`${V1}/habits?all=1`).then(setHabits).catch(() => {})
    api.get<Tracker[]>(`${V1}/trackers?all=1`).then(setTrackers).catch(() => {})
  }, [])

  const commands: Command[] = useMemo(() => [
    { id: 'today', label: "Today's check-in", icon: 'check-square', hint: 'T', keywords: 'check in mark', run: () => navigate('/today') },
    { id: 'yesterday', label: "Yesterday's check-in", icon: 'undo', keywords: 'check in mark', run: () => navigate(`/today/${shiftDate(todayISO(), -1)}`) },
    { id: 'month', label: 'This month', icon: 'grid', hint: 'M', keywords: 'month page grid', run: () => navigate('/month') },
    { id: 'prev', label: `Last month — ${monthTitle(shiftMonth(currentMonth(), -1))}`, icon: 'chevron-left', keywords: 'previous month', run: () => navigate(`/month/${shiftMonth(currentMonth(), -1)}`) },
    { id: 'insights', label: 'Insights', icon: 'chart', hint: 'I', keywords: 'stats correlations weather', run: () => navigate('/insights') },
    { id: 'manage', label: 'Habits & trackers', icon: 'list', hint: 'H', keywords: 'manage edit', run: () => navigate('/manage') },
    { id: 'new-habit', label: 'New habit', icon: 'plus', hint: 'N', keywords: 'add create', run: () => navigate('/manage?new=habit') },
    { id: 'new-tracker', label: 'New tracker', icon: 'sliders', keywords: 'add create score measurement yes no', run: () => navigate('/manage?new=tracker') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'theme', label: theme === 'dark' ? 'Switch to light' : 'Switch to dark', icon: theme === 'dark' ? 'sun' : 'moon', keywords: 'theme appearance', run: () => setThemeMode(theme === 'dark' ? 'light' : 'dark') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: onShortcuts },
  ], [navigate, theme, setThemeMode, onShortcuts])

  type Row = { key: string; group: string; icon: React.ReactNode; label: React.ReactNode; sub?: React.ReactNode; hint?: string; run: () => void }
  const rows = useMemo<Row[]>(() => {
    const out: Row[] = []
    const n = q.trim().toLowerCase()
    const words = n.split(/\s+/).filter(Boolean)
    const hit = (text: string) => words.every(w => text.toLowerCase().includes(w))
    // "2026-03-14" → that day, "2026-03" → that month
    if (/^\d{4}-\d{2}-\d{2}$/.test(n) && !Number.isNaN(Date.parse(n))) out.push({ key: 'day', group: 'Go to', icon: <Icon name="calendar" size={16} />, label: `Check-in for ${longDate(n)}`, run: () => navigate(`/today/${n}`) })
    else if (/^\d{4}-\d{2}$/.test(n)) out.push({ key: 'mon', group: 'Go to', icon: <Icon name="grid" size={16} />, label: monthTitle(n), run: () => navigate(`/month/${n}`) })
    else if (n.length >= 3) {
      // a month name: the last twelve months
      for (let i = 0; i < 12; i++) {
        const m = shiftMonth(currentMonth(), -i)
        if (hit(monthTitle(m))) out.push({ key: 'm' + m, group: 'Go to', icon: <Icon name="grid" size={16} />, label: monthTitle(m), run: () => navigate(`/month/${m}`) })
      }
    }
    const today = todayISO()
    for (const h of habits) {
      if (n && !hit(h.name)) continue
      const ended = !!h.end_date && h.end_date < today
      out.push({ key: `h${h.id}`, group: 'Habits', icon: <span className="pal-dot" style={{ background: h.color || 'var(--border-strong)' }} />, label: h.name, sub: ended ? `ended ${h.end_date}` : undefined, run: () => navigate(`/manage?habit=${h.id}`) })
    }
    for (const t of trackers) {
      if (n && !hit(`${t.name} tracker`)) continue
      const ended = !!t.end_date && t.end_date < today
      out.push({ key: `t${t.id}`, group: 'Trackers', icon: <Icon name="sliders" size={16} />, label: t.name, sub: [t.kind === 'scale' ? `score ${t.min_value}–${t.max_value}` : t.kind === 'yesno' ? 'yes / no' : `measurement${t.unit ? ' · ' + t.unit : ''}`, ended && `ended ${t.end_date}`].filter(Boolean).join(' · '), run: () => navigate(`/manage?tracker=${t.id}`) })
    }
    for (const c of commands) {
      if (n && !hit(`${c.label} ${c.keywords || ''}`)) continue
      out.push({ key: c.id, group: 'Commands', icon: <Icon name={c.icon} size={16} />, label: c.label, hint: c.hint, run: c.run })
    }
    // with nothing typed, commands first
    if (!n) out.sort((a, b) => (a.group === 'Commands' ? 0 : 1) - (b.group === 'Commands' ? 0 : 1))
    return out
  }, [q, habits, trackers, commands, navigate])
  useEffect(() => setHi(0), [q])
  useEffect(() => { list.current?.querySelector(`[data-i="${hi}"]`)?.scrollIntoView({ block: 'nearest' }) }, [hi])

  const run = (r: Row | undefined) => { if (!r) return; onClose(); r.run() }
  let last = ''
  return createPortal(
    <div className="palette-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="palette-box" role="dialog" aria-label="Search and commands">
        <div className="pal-input">
          <Icon name="search" size={18} />
          <input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Jump to a habit, a month (or 2026-03-14), or run a command…" aria-label="Search"
            onKeyDown={e => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setHi(h => Math.min(rows.length - 1, h + 1)) }
              else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)) }
              else if (e.key === 'Enter') { e.preventDefault(); run(rows[hi]) }
              else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onClose() }
            }} />
          <kbd>esc</kbd>
        </div>
        <div className="pal-list" ref={list} role="listbox">
          {rows.length === 0 && <div className="pal-empty">Nothing matches.</div>}
          {rows.map((r, i) => {
            const head = r.group !== last ? <div className="pal-group" key={'g' + r.group}>{r.group}</div> : null
            last = r.group
            return (
              <React.Fragment key={r.key}>
                {head}
                <div data-i={i} role="option" aria-selected={i === hi} className={`pal-row ${i === hi ? 'hi' : ''}`} onMouseEnter={() => setHi(i)} onClick={() => run(r)}>
                  <span className="pal-ico">{r.icon}</span>
                  <span className="grow"><span className="pal-label">{r.label}</span>{r.sub && <span className="pal-sub">{r.sub}</span>}</span>
                  {r.hint && <kbd>{r.hint}</kbd>}
                  {i === hi && <Icon name="chevron-right" size={14} className="muted" />}
                </div>
              </React.Fragment>
            )
          })}
        </div>
      </div>
    </div>, document.body)
}
