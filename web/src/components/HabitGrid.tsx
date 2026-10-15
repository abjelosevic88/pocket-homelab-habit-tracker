import type { Day, MonthHabit, WeatherDay } from '../lib/types'
import { isoWeekday, shortDate } from '../lib/dates'
import { CONDITION_ICON, temp, weatherLine } from './Weather'

interface Props {
  days: Day[]
  habits: MonthHabit[]
  onCycle: (habit: MonthHabit, date: string, next: number) => void
  onEdit?: (habit: MonthHabit) => void
  weather?: Record<string, WeatherDay> | null
  units?: string
  onWeather?: (day: Day, w: WeatherDay | undefined) => void
  /** Important notes by date: those days are drawn red. */
  important?: Record<string, string> | null
  /** A click on a day's number, to write or read its important note. */
  onDay?: (day: Day) => void
}

// Column widths shared with the tracker charts drawn under the grid; the CSS
// min-widths match, the table's explicit width makes table-layout: fixed hold them.
export const COL = 28
export const NAME_W = 160
export const SUM_W = 64
export const sheetWidth = (days: number) => NAME_W + days * COL + SUM_W

// Click cycles blank -> done (green) -> missed (red) -> blank, like two pens.
export function nextStatus(cur: number): number { return cur === 0 ? 1 : cur === 1 ? -1 : 0 }

export default function HabitGrid({ days, habits, onCycle, onEdit, weather, units = 'celsius', onWeather, important, onDay }: Props) {
  const hasWeather = !!weather && Object.keys(weather).length > 0
  return (
  <table className="habit-grid" style={{ width: sheetWidth(days.length) }}>
      <thead>
        <tr>
          <th className="name-col" />
          {days.map(d => {
            const note = important?.[d.date]
            return (
              <th key={d.date} className={`day ${d.weekend ? 'weekend' : ''} ${d.today ? 'today' : ''} ${note ? 'important' : ''}`}>
                <button type="button" className="day-btn" onClick={() => onDay?.(d)} title={note ? `${shortDate(d.date)} · ${note}` : `${shortDate(d.date)} · click to add an important note`}>
                  <span className="num">{d.day}</span><span className="wd">{d.label}</span>
                </button>
              </th>
            )
          })}
          <th className="sum-col">✓ / ✗</th>
        </tr>
        {hasWeather && (
          <tr className="wx-row">
            <th className="name-col" scope="row"><span className="wx-label">Weather</span></th>
            {days.map(d => {
              const w = weather?.[d.date]
              return (
                <th key={d.date} className={`day wx ${d.today ? 'today' : ''}`}>
                  <button type="button" className="wx-cell" disabled={d.future} title={w ? weatherLine(w, units) : d.future ? '' : 'No weather for this day yet — click to set it'} onClick={() => onWeather?.(d, w)}>
                    <span className="wx-i">{w ? (CONDITION_ICON[w.condition] || '·') : ''}</span>
                    <span className="wx-t">{w && w.t_max !== null ? temp(w.t_max, units) : ''}</span>
                  </button>
                </th>
              )
            })}
            <th className="sum-col" />
          </tr>
        )}
      </thead>
      <tbody>
        {habits.map(h => (
          <tr key={h.id}>
            <th className="name-col" scope="row">
              <button className="name-btn" onClick={() => onEdit?.(h)} title={h.days === '1234567' ? 'Every day' : 'Scheduled on selected days'}>
                {h.color && <span className="swatch" style={{ background: h.color }} />}
                <span className="nm">{h.name}</span>
                {h.streak > 1 && <span className="streak" title={`${h.streak}-day streak`}>🔥{h.streak}</span>}
              </button>
            </th>
            {days.map(d => {
              const st = h.entries[d.date] || 0
              const wd = isoWeekday(d.date)
              const scheduled = h.days.includes(String(wd)) && d.date >= h.start_date && (!h.end_date || d.date <= h.end_date)
              const cls = ['cell', st === 1 ? 'done' : st === -1 ? 'missed' : '', !scheduled ? 'off' : '', d.future ? 'future' : '', d.today ? 'today' : ''].join(' ')
              return (
                <td key={d.date} className={cls}>
                  <button
                    disabled={d.future}
                    aria-label={`${h.name} ${shortDate(d.date)}: ${st === 1 ? 'done' : st === -1 ? 'missed' : 'not marked'}`}
                    title={`${shortDate(d.date)} · ${st === 1 ? 'done' : st === -1 ? 'missed' : scheduled ? 'not marked' : 'not scheduled'}`}
                    onClick={() => onCycle(h, d.date, nextStatus(st))}
                  >
                    {st === 1 ? '✓' : st === -1 ? '✗' : ''}
                  </button>
                </td>
              )
            })}
            <td className="sum-col"><span className="ok">{h.done}</span><span className="muted"> / </span><span className="bad">{h.missed}</span></td>
          </tr>
        ))}
      </tbody>
  </table>
  )
}
