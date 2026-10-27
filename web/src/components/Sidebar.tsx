// Left column: check-in button, sections, today's habits, settings — the Pocket Calendar sidebar.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { todayISO } from '../lib/dates'
import type { TodayHabit, TodayPage } from '../lib/types'
import { Icon } from './icons'
import { announceChange, openShortcuts } from './Layout'
import { nextStatus } from './HabitGrid'
import { type Anchor, type MenuItem, Menu, rectOf, useToast } from './ui'

/** Today's page (habits with their marks), re-read whenever something is saved. */
function useTodayPage() {
  const [page, setPage] = useState<TodayPage | null>(null)
  const timer = useRef<number>(0)
  const load = useCallback(() => { api.get<TodayPage>(`${V1}/days/${todayISO()}`).then(setPage).catch(() => {}) }, [])
  useEffect(() => {
    load()
    const soon = () => { clearTimeout(timer.current); timer.current = window.setTimeout(load, 250) }
    const vis = () => { if (document.visibilityState === 'visible') load() }
    window.addEventListener('ph:mutated', soon)
    document.addEventListener('visibilitychange', vis)
    return () => { clearTimeout(timer.current); window.removeEventListener('ph:mutated', soon); document.removeEventListener('visibilitychange', vis) }
  }, [load])
  return { page, setPage, load }
}

export function Sidebar({ onNavigate }: { onNavigate: () => void }) {
  const { status, settings } = useApp()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { page, setPage, load } = useTodayPage()
  const title = settings?.title || status?.title || 'Pocket Habits'
  const scheduled = page?.habits.filter(h => h.scheduled) || []
  const left = scheduled.filter(h => h.status === 0).length

  const mark = async (h: TodayHabit, next: number) => {
    setPage(p => p && { ...p, habits: p.habits.map(x => x.id === h.id ? { ...x, status: next } : x) })
    try { await api.put(`${V1}/habits/${h.id}/entries/${todayISO()}`, { status: next }); announceChange() } catch (e) { toast((e as Error).message, 'err'); load() }
  }

  const nav = [
    { to: '/today', label: 'Today', icon: 'check-square', badge: left, badgeTitle: `${left} left to mark today` },
    { to: '/month', label: 'Month', icon: 'grid' },
    { to: '/insights', label: 'Insights', icon: 'chart' },
    { to: '/manage', label: 'Habits & trackers', icon: 'list' },
  ]
  return (
    <aside className="sidebar" aria-label="Habits">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Habits' ? title : <>Pocket <b>Habits</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" title="Today's check-in (t)" onClick={() => { onNavigate(); navigate('/today') }}>
        <Icon name="check" size={17} strokeWidth={2.4} />Check in
        {page && scheduled.length > 0 && <span className="create-count">{scheduled.length - left}/{scheduled.length}</span>}
      </button>

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}{!!n.badge && <span className="badge accent" title={n.badgeTitle}>{n.badge}</span>}
          </NavLink>
        ))}
      </nav>

      <HabitGroup habits={page?.habits || null} onMark={mark} onNavigate={onNavigate} />

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); openShortcuts() }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}

function HabitGroup({ habits, onMark, onNavigate }: { habits: TodayHabit[] | null; onMark: (h: TodayHabit, next: number) => void; onNavigate: () => void }) {
  const [open, setOpen] = useState(true)
  const navigate = useNavigate()
  const add = () => { onNavigate(); navigate('/manage?new=habit') }
  return (
    <section className="sb-sec">
      <h4 className="sb-h">
        <button type="button" className="sb-h-btn" aria-expanded={open} onClick={() => setOpen(o => !o)}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={13} />Today's habits</button>
        <button type="button" className="btn ghost icon-only xs" aria-label="Add a habit" title="Add a habit (n)" onClick={add}><Icon name="plus" size={14} /></button>
      </h4>
      {open && habits && (habits.length === 0
        ? <p className="sb-empty">No habits yet. <button type="button" className="link-btn" onClick={add}>Add one</button></p>
        : <ul className="cal-list">{habits.map(h => <HabitRow key={h.id} h={h} onMark={onMark} onNavigate={onNavigate} />)}</ul>)}
    </section>
  )
}

function HabitRow({ h, onMark, onNavigate }: { h: TodayHabit; onMark: (h: TodayHabit, next: number) => void; onNavigate: () => void }) {
  const navigate = useNavigate()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const go = (to: string) => { onNavigate(); navigate(to) }
  const items: MenuItem[] = [
    { heading: 'Today' },
    { label: 'Done', icon: 'check', checked: h.status === 1 ? true : undefined, onSelect: () => onMark(h, h.status === 1 ? 0 : 1) },
    { label: 'Missed', icon: 'x', checked: h.status === -1 ? true : undefined, onSelect: () => onMark(h, h.status === -1 ? 0 : -1) },
    ...(h.status !== 0 ? [{ label: 'Clear', icon: 'undo', onSelect: () => onMark(h, 0) }] : []),
    'sep',
    { label: 'Edit habit', icon: 'edit', onSelect: () => go(`/manage?habit=${h.id}`) },
    { label: 'Month page', icon: 'grid', onSelect: () => go('/month') },
  ]
  const state = h.status === 1 ? 'done' : h.status === -1 ? 'missed' : ''
  const label = h.status === 1 ? 'done' : h.status === -1 ? 'missed' : h.scheduled ? 'not marked' : 'not scheduled today'
  return (
    <li className={`cal-row hab-row ${h.scheduled ? '' : 'unscheduled'}`}>
      <button type="button" className="cal-check" onClick={() => go(`/manage?habit=${h.id}`)} title={`${h.name} — edit`}>
        <span className="hab-swatch" style={{ ['--c' as string]: h.color || undefined }} />
        <span className="cal-name ellipsis">{h.name}</span>
        {h.streak > 1 && <span className="hab-meta" title={`${h.streak}-day streak`}><Icon name="flame" size={12} strokeWidth={2} />{h.streak}</span>}
      </button>
      <button type="button" className="btn ghost icon-only xs cal-more" aria-label={`Options for ${h.name}`} aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(rectOf(e.currentTarget))}><Icon name="more" size={15} /></button>
      <button type="button" className={`hab-state ${state} ${h.scheduled ? '' : 'off'}`} aria-label={`${h.name} today: ${label} — click to change`} title={`Today: ${label} · click: done → missed → clear`} onClick={() => onMark(h, nextStatus(h.status))}>
        {h.status === 1 ? <Icon name="check" size={12} strokeWidth={3} /> : h.status === -1 ? <Icon name="x" size={12} strokeWidth={3} /> : null}
      </button>
      {menu && <Menu anchor={menu} items={items} onClose={() => setMenu(null)} width={200} />}
    </li>
  )
}
