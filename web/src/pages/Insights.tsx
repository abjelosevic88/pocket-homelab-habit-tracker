import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Insights as InsightsT, TrackerInsight, WeatherBucket } from '../lib/types'
import { CONDITION_ICON } from '../components/Weather'
import { WEEKDAY_LETTER, fmtNum, pct, shortDate } from '../lib/dates'
import { Card, Empty, Loading, PageHeader, Segmented, useAsync } from '../components/ui'

export default function Insights() {
  const [months, setMonths] = useState(3)
  const { data, loading, error } = useAsync(() => api.get<InsightsT>(`${V1}/insights?months=${months}`), [months])
  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const scales = data.trackers.filter(t => t.kind === 'scale')
  const numbers = data.trackers.filter(t => t.kind === 'number')
  const yesnos = data.trackers.filter(t => t.kind === 'yesno')
  const strong = data.correlations.filter(c => c.delta !== null && c.n_done >= 3 && c.n_missed >= 3)

  return (
    <>
      <PageHeader title="Insights" sub={`${data.from} → ${data.to}. See how sleep and mood line up with what you did or didn't do.`} actions={<Segmented value={months} onChange={setMonths} options={[{ value: 1, label: '1 mo' }, { value: 3, label: '3 mo' }, { value: 6, label: '6 mo' }, { value: 12, label: '12 mo' }]} />} />

      <Card title="Habits" flush className="mb">
        {data.habits.length === 0 ? <div className="pad"><Empty title="Nothing to analyse yet"><Link to="/manage">Add habits</Link> and mark a few days.</Empty></div> : (
          <table className="table">
            <thead><tr><th>Habit</th><th>Rate</th><th className="right">✓</th><th className="right">✗</th><th className="right">Streak</th><th className="right">Best</th><th>By weekday</th></tr></thead>
            <tbody>
              {data.habits.map(h => (
                <tr key={h.id} className={h.end_date && h.end_date < data.to ? 'muted' : ''}>
                  <td><span className="row">{h.color && <span className="swatch" style={{ background: h.color }} />}{h.name}</span></td>
                  <td style={{ minWidth: 140 }}><div className="bar"><span className="fill ok" style={{ width: pct(h.rate) === '–' ? 0 : pct(h.rate) }} /></div><span className="small">{pct(h.rate)}</span></td>
                  <td className="right ok">{h.done}</td>
                  <td className="right bad">{h.missed}</td>
                  <td className="right">{h.streak > 0 ? `🔥${h.streak}` : '–'}</td>
                  <td className="right">{h.best_streak}</td>
                  <td><div className="wd-heat">{h.weekdays.map((r, i) => <span key={i} className={`wd ${r === null ? 'na' : r >= 2 / 3 ? 'high' : r >= 1 / 3 ? 'mid' : 'low'}`} title={`${WEEKDAY_LETTER[i]}: ${pct(r)}`}>{WEEKDAY_LETTER[i]}</span>)}</div></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <div className="two-col">
        <Card title="What moves your scores" className="mb">
          {strong.length === 0 ? <p className="muted">Needs at least three done and three missed days per habit alongside a score. Keep going.</p> : (
            <table className="table compact">
              <thead><tr><th>Score</th><th>Habit / question</th><th className="right">with</th><th className="right">without</th><th className="right">Δ</th></tr></thead>
              <tbody>
                {strong.slice(0, 12).map(c => (
                  <tr key={`${c.tracker_id}-${c.habit_id}`}>
                    <td>{c.tracker_name}</td>
                    <td>{c.habit_name}{c.kind === 'yesno' && <span className="muted small"> (yes vs no)</span>}</td>
                    <td className="right">{fmtNum(c.avg_done)} <span className="muted small">({c.n_done})</span></td>
                    <td className="right">{fmtNum(c.avg_missed)} <span className="muted small">({c.n_missed})</span></td>
                    <td className={`right ${(c.delta ?? 0) > 0 ? 'ok' : (c.delta ?? 0) < 0 ? 'bad' : ''}`}>{(c.delta ?? 0) > 0 ? '+' : ''}{fmtNum(c.delta)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p className="muted small mt">Average score on days the habit was green vs red, or a yes/no question was yes vs no. A positive Δ means higher scores with it. Correlation, not proof.</p>
        </Card>

        <div>
          {scales.map(t => <Sparkline key={t.id} t={t} />)}
          {numbers.map(t => <Sparkline key={t.id} t={t} />)}
          {yesnos.map(t => <YesNoStrip key={t.id} t={t} />)}
        </div>
      </div>

      {data.weather && data.weather.days > 0 && <WeatherCard w={data.weather} />}

      {data.journal.length > 0 && (
        <Card title="Good things" actions={<span className="muted small">{data.journal.length} entries</span>}>
          <ul className="journal-read">
            {data.journal.slice(0, 60).map(j => <li key={j.date}><span className="jdate">{shortDate(j.date)}</span><span>{j.text && <span>{j.text}</span>}{j.bad && <span className="bad-line">{j.bad}</span>}</span></li>)}
          </ul>
        </Card>
      )}
    </>
  )
}

function WeatherCard({ w }: { w: import('../lib/types').WeatherInsights }) {
  const groups: { title: string; rows: WeatherBucket[] }[] = [
    { title: 'By weather', rows: w.by_condition },
    { title: 'By high temperature', rows: w.by_temperature },
    { title: 'By daylight', rows: w.by_daylight },
  ].filter(g => g.rows.length > 0)
  const scoreNames = w.by_condition[0]?.scores.map(s => s.tracker_name) ?? w.by_temperature[0]?.scores.map(s => s.tracker_name) ?? []
  return (
    <Card title="Weather and your scores" className="mb" actions={<span className="muted small">{w.days} days with weather</span>}>
      {groups.map(g => (
        <div key={g.title} className="mb">
          <div className="small muted" style={{ marginBottom: 4 }}>{g.title}</div>
          <table className="table compact">
            <thead><tr><th /><th className="right">Days</th>{scoreNames.map(n => <th key={n} className="right">{n}</th>)}<th className="right">Habits ✓</th></tr></thead>
            <tbody>
              {g.rows.map(b => (
                <tr key={b.key}>
                  <td>{CONDITION_ICON[b.key] ? <span>{CONDITION_ICON[b.key]} </span> : null}{b.label}</td>
                  <td className="right">{b.days}</td>
                  {b.scores.map(s => <td key={s.tracker_id} className="right">{fmtNum(s.avg)}{s.n ? <span className="muted small"> ({s.n})</span> : ''}</td>)}
                  <td className="right">{pct(b.habit_rate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
      <p className="muted small">Each score's average on the days of that kind, and how often habits were green. The daylight rows carry the seasonal signal. Correlation, not proof.</p>
    </Card>
  )
}

function YesNoStrip({ t }: { t: TrackerInsight }) {
  const yes = t.series.filter(p => p.value === 1).length
  const yesTone = t.higher_is_better ? 'good' : 'bad', noTone = t.higher_is_better ? 'bad' : 'good'
  return (
    <Card title={t.name} className="mb" actions={<span className="small muted">{yes} yes of {t.series.length} days ({t.series.length ? Math.round(yes / t.series.length * 100) : 0}%)</span>}>
      {t.series.length === 0 ? <p className="muted">No answers yet.</p> : (
        <div className="yn-strip">{t.series.map(p => <span key={p.date} className={`yn ${p.value === 1 ? yesTone : noTone}`} title={`${shortDate(p.date)}: ${p.value === 1 ? 'yes' : 'no'}`} />)}</div>
      )}
      <div className="row" style={{ justifyContent: 'space-between' }}><span className="muted small">{t.series.length ? shortDate(t.series[0].date) : ''}</span><span className="muted small">one square per day · green = good answer</span><span className="muted small">{t.series.length ? shortDate(t.series[t.series.length - 1].date) : ''}</span></div>
    </Card>
  )
}

function Sparkline({ t }: { t: TrackerInsight }) {
  const W = 520, H = 90, P = 6
  const s = t.series
  if (s.length === 0) return <Card title={t.name} className="mb"><p className="muted">No values yet.</p></Card>
  const vals = s.map(p => p.value)
  const lo = t.kind === 'scale' ? t.min_value : Math.min(...vals), hi = t.kind === 'scale' ? t.max_value : Math.max(...vals)
  const span = hi - lo || 1
  const x = (i: number) => P + (i / Math.max(1, s.length - 1)) * (W - 2 * P)
  const y = (v: number) => P + (1 - (v - lo) / span) * (H - 2 * P)
  const d = s.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(i)},${y(p.value)}`).join(' ')
  // 7-point moving average
  const ma = s.map((_, i) => { const w = s.slice(Math.max(0, i - 6), i + 1); return w.reduce((a, b) => a + b.value, 0) / w.length })
  const dma = ma.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i)},${y(v)}`).join(' ')
  const delta = t.first !== null && t.last !== null ? t.last - t.first : null
  return (
    <Card title={t.name} className="mb" actions={<span className="small muted">avg {fmtNum(t.avg)}{t.unit ? ' ' + t.unit : ''}{delta !== null && t.kind === 'number' ? ` · ${delta >= 0 ? '+' : ''}${fmtNum(delta)} over range` : ''}</span>}>
      <svg viewBox={`0 0 ${W} ${H}`} className="spark" role="img" aria-label={`${t.name} over time`}>
        {t.kind === 'scale' && <>
          <rect className={`band ${t.higher_is_better ? 'high' : 'low'}`} x={0} y={0} width={W} height={H / 3} />
          <rect className="band mid" x={0} y={H / 3} width={W} height={H / 3} />
          <rect className={`band ${t.higher_is_better ? 'low' : 'high'}`} x={0} y={2 * H / 3} width={W} height={H / 3} />
        </>}
        <path className="line faint" d={d} />
        <path className="line" d={dma} />
        <circle className="pt" cx={x(s.length - 1)} cy={y(s[s.length - 1].value)} r={4} />
      </svg>
      <div className="row" style={{ justifyContent: 'space-between' }}><span className="muted small">{shortDate(s[0].date)}</span><span className="muted small">bold = 7-day average</span><span className="muted small">{shortDate(s[s.length - 1].date)}</span></div>
    </Card>
  )
}
