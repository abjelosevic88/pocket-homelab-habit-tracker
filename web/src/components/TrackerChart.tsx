import { useState } from 'react'
import type { Day, MonthTracker } from '../lib/types'
import { bandFor } from './ui'
import { fmtNum, shortDate } from '../lib/dates'

interface Props {
  days: Day[]
  tracker: MonthTracker
  onSet: (tracker: MonthTracker, date: string, value: number | null) => void
}

const COL = 28     // must match .habit-grid cell width
const NAME = 160   // must match .name-col width
const SUM = 64     // must match .sum-col width
const H = 96
const PAD_T = 8
const PAD_B = 8

// One row of the notebook: a scale tracker draws over three highlighter bands,
// a number tracker (weight) is a plain line that auto-scales.
export default function TrackerChart({ days, tracker, onSet }: Props) {
  const [pick, setPick] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  const isScale = tracker.kind === 'scale'
  const isYesNo = tracker.kind === 'yesno'
  const vals = days.map(d => tracker.entries[d.date]).filter(v => v !== undefined)
  let lo = tracker.min_value, hi = tracker.max_value
  if (!isScale) {
    if (vals.length) { lo = Math.min(...vals); hi = Math.max(...vals) }
    else { lo = 0; hi = 1 }
    const span = Math.max(hi - lo, isScale ? 1 : Math.max(1, hi * 0.02))
    lo = lo - span * 0.15; hi = hi + span * 0.15
  }
  const y = (v: number) => PAD_T + (1 - (v - lo) / (hi - lo || 1)) * (H - PAD_T - PAD_B)
  const x = (i: number) => NAME + i * COL + COL / 2
  const width = NAME + days.length * COL + SUM
  const pts: { i: number; v: number; d: Day }[] = []
  days.forEach((d, i) => { const v = tracker.entries[d.date]; if (v !== undefined) pts.push({ i, v, d }) })
  const path = pts.map((p, k) => `${k === 0 ? 'M' : 'L'}${x(p.i)},${y(p.v)}`).join(' ')

  const steps: number[] = []
  if (isScale) for (let v = tracker.min_value; v <= tracker.max_value + 1e-9; v += tracker.step) steps.push(Number(v.toFixed(4)))

  const cycleYesNo = (d: Day) => {
    if (d.future) return
    const cur = tracker.entries[d.date]
    onSet(tracker, d.date, cur === undefined ? 1 : cur === 1 ? 0 : null)
  }
  const open = (d: Day) => {
    if (d.future) return
    if (isYesNo) { cycleYesNo(d); return }
    setPick(d.date)
    setDraft(tracker.entries[d.date] !== undefined ? String(tracker.entries[d.date]) : '')
  }
  const commit = (date: string, raw: string) => {
    const v = raw.trim() === '' ? null : Number(raw.replace(',', '.'))
    if (v !== null && Number.isNaN(v)) return
    onSet(tracker, date, v)
    setPick(null)
  }
  const bandY = (frac: number) => PAD_T + (1 - frac) * (H - PAD_T - PAD_B)

  if (isYesNo) {
    const yes = pts.filter(p => p.v === 1).length
    const YH = 52
    const yesTone = tracker.higher_is_better ? 'good' : 'bad', noTone = tracker.higher_is_better ? 'bad' : 'good'
    return (
      <div className="tracker-row yesno">
        <div className="grid-scroll">
          <svg className="tracker-svg" width={width} height={YH} role="img" aria-label={`${tracker.name} this month`}>
            {days.map((d, i) => d.weekend ? <rect key={d.date} className="weekend-col" x={NAME + i * COL} y={0} width={COL} height={YH} /> : null)}
            {days.map((d, i) => d.today ? <rect key={d.date + 't'} className="today-col" x={NAME + i * COL} y={0} width={COL} height={YH} /> : null)}
            <foreignObject x={0} y={0} width={NAME} height={YH}>
              <div className="tracker-label"><strong>{tracker.name}</strong><span className="muted small">yes / no</span></div>
            </foreignObject>
            {days.map((d, i) => {
              const v = tracker.entries[d.date]
              const dflt = tracker.defaulted?.[d.date]
              const cls = `${v === 1 ? `yn-dot ${yesTone}` : v === 0 ? `yn-dot ${noTone} no` : 'yn-dot empty'} ${dflt ? 'implicit' : ''}`
              return <circle key={d.date} className={cls} cx={x(i)} cy={YH / 2} r={v === undefined ? 3 : dflt ? 6 : 7}><title>{`${shortDate(d.date)}: ${v === 1 ? 'yes' : v === 0 ? 'no' : d.future ? '' : 'not marked — click to cycle'}${dflt ? ' (by default)' : ''}`}</title></circle>
            })}
            {days.map((d, i) => <rect key={d.date + 'h'} className={`hit ${d.future ? 'future' : ''}`} x={NAME + i * COL} y={0} width={COL} height={YH} onClick={() => open(d)} />)}
            <foreignObject x={NAME + days.length * COL} y={0} width={SUM} height={YH}>
              <div className="tracker-sum"><strong>{yes}</strong><span className="muted small">yes</span></div>
            </foreignObject>
          </svg>
        </div>
      </div>
    )
  }

  return (
    <div className="tracker-row">
      <div className="grid-scroll">
        <svg className="tracker-svg" width={width} height={H} role="img" aria-label={`${tracker.name} this month`}>
          {/* bands (scale) or baseline grid (number) */}
          {isScale ? (
            <>
              <rect className="band high" x={NAME} y={bandY(1)} width={days.length * COL} height={bandY(2 / 3) - bandY(1)} />
              <rect className="band mid" x={NAME} y={bandY(2 / 3)} width={days.length * COL} height={bandY(1 / 3) - bandY(2 / 3)} />
              <rect className="band low" x={NAME} y={bandY(1 / 3)} width={days.length * COL} height={bandY(0) - bandY(1 / 3)} />
            </>
          ) : (
            [0.25, 0.5, 0.75].map(f => <line key={f} className="gridline" x1={NAME} x2={NAME + days.length * COL} y1={bandY(f)} y2={bandY(f)} />)
          )}
          {!tracker.higher_is_better && isScale && <title>Lower is better</title>}
          {/* weekend tint + today marker */}
          {days.map((d, i) => d.weekend ? <rect key={d.date} className="weekend-col" x={NAME + i * COL} y={PAD_T} width={COL} height={H - PAD_T - PAD_B} /> : null)}
          {days.map((d, i) => d.today ? <rect key={d.date + 't'} className="today-col" x={NAME + i * COL} y={0} width={COL} height={H} /> : null)}
          {/* row label (left, sticky look) */}
          <foreignObject x={0} y={0} width={NAME} height={H}>
            <div className="tracker-label">
              <strong>{tracker.name}</strong>
              <span className="muted small">{isScale ? `${fmtNum(tracker.min_value)}–${fmtNum(tracker.max_value)}` : tracker.unit}</span>
              {isScale && steps.length <= 7 && <span className="scale-ticks">{[...steps].reverse().map(s => <i key={s}>{s}</i>)}</span>}
            </div>
          </foreignObject>
          {/* line + points */}
          {pts.length > 1 && <path className="line" d={path} />}
          {pts.map(p => (
            <circle key={p.d.date} className={`pt ${isScale ? bandFor(p.v, tracker.min_value, tracker.max_value, tracker.higher_is_better) : ''}`} cx={x(p.i)} cy={y(p.v)} r={4.5}>
              <title>{`${shortDate(p.d.date)}: ${fmtNum(p.v)}${tracker.unit ? ' ' + tracker.unit : ''}`}</title>
            </circle>
          ))}
          {/* hit targets */}
          {days.map((d, i) => (
            <rect key={d.date + 'h'} className={`hit ${d.future ? 'future' : ''}`} x={NAME + i * COL} y={0} width={COL} height={H} onClick={() => open(d)}>
              <title>{`${shortDate(d.date)}${tracker.entries[d.date] !== undefined ? `: ${fmtNum(tracker.entries[d.date])}${tracker.unit ? ' ' + tracker.unit : ''}` : d.future ? '' : ' — click to record'}`}</title>
            </rect>
          ))}
          {/* summary */}
          <foreignObject x={NAME + days.length * COL} y={0} width={SUM} height={H}>
            <div className="tracker-sum">
              {isScale
                ? <><span className="muted small">avg</span><strong>{fmtNum(tracker.avg)}</strong></>
                : <><span className="muted small">{tracker.first !== null && tracker.last !== null ? 'Δ' : ''}</span><strong>{tracker.first !== null && tracker.last !== null ? `${tracker.last - tracker.first >= 0 ? '+' : ''}${fmtNum(tracker.last - tracker.first)}` : '–'}</strong><span className="muted small">{tracker.last !== null ? `${fmtNum(tracker.last)} ${tracker.unit}` : ''}</span></>}
            </div>
          </foreignObject>
        </svg>
      </div>
      {pick && (
        <div className="picker-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) setPick(null) }}>
          <div className="picker">
            <div className="picker-head"><strong>{tracker.name}</strong><span className="muted">{shortDate(pick)}</span></div>
            {isScale ? (
              <div className="pills">
                {steps.map(s => <button key={s} className={`pill ${bandFor(s, tracker.min_value, tracker.max_value, tracker.higher_is_better)} ${tracker.entries[pick] === s ? 'active' : ''}`} onClick={() => commit(pick, String(s))}>{fmtNum(s)}</button>)}
              </div>
            ) : (
              <form className="row" onSubmit={e => { e.preventDefault(); commit(pick, draft) }}>
                <input autoFocus inputMode="decimal" value={draft} onChange={e => setDraft(e.target.value)} placeholder={tracker.unit} />
                <button className="btn primary">Save</button>
              </form>
            )}
            <div className="row" style={{ justifyContent: 'space-between', marginTop: 10 }}>
              <button className="btn ghost sm" onClick={() => commit(pick, '')} disabled={tracker.entries[pick] === undefined}>Clear</button>
              <button className="btn ghost sm" onClick={() => setPick(null)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
