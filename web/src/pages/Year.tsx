// Your year outdoors: how far, how high, how long, month by month, by activity.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate } from '../lib/format'
import { ACTIVITY, distUnit, fmtDist, fmtHeight, fmtMinutes, outingTitle } from '../lib/trails'
import type { Activity, YearTotals } from '../lib/types'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Loading, useAsync } from '../components/ui'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export default function Year() {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const [year, setYear] = useState(new Date().getFullYear())
  const { data } = useAsync(() => api.get<{ year: YearTotals; previous: YearTotals; years: number[] }>(`${V1}/stats?year=${year}`), [year])
  if (!data) return <Loading />
  const y = data.year, p = data.previous
  const years = Array.from(new Set([new Date().getFullYear(), ...data.years])).sort((a, b) => b - a)
  const du = distUnit(u)
  const maxD = Math.max(1, ...y.by_month.map(m => m.distance_m))
  const delta = (a: number, b: number) => b > 0 ? Math.round(((a - b) / b) * 100) : null
  const dd = delta(y.distance_m, p.distance_m)
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Your year</h1><p className="muted">{y.outings ? `${y.outings} ${y.outings === 1 ? 'outing' : 'outings'} done in ${y.year}${dd != null ? ` · ${dd >= 0 ? '+' : ''}${dd}% distance on ${y.year - 1}` : ''}` : `Nothing done in ${y.year} yet.`}</p></div>
        <Select ariaLabel="Year" value={String(year)} onChange={v => setYear(Number(v))} options={years.map(n => ({ value: String(n), label: String(n) }))} />
      </div>
      <div className="tiles">
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="route" size={14} /></span>Distance</div><div className="tile-val small-val">{fmtDist(y.distance_m, u)}</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="mountain" size={14} /></span>Climbed</div><div className="tile-val small-val">{fmtHeight(y.gain_m, u)}</div><div className="muted small">{y.gain_m > 8849 ? `${(y.gain_m / 8849).toFixed(1)}× Everest` : ''}</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="clock" size={14} /></span>Time out</div><div className="tile-val small-val">{fmtMinutes(y.minutes)}</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="flag" size={14} /></span>Summits</div><div className="tile-val">{y.summits}</div><div className="muted small">{y.highest != null ? `highest ${fmtHeight(y.highest, u)}` : ''}</div></div>
      </div>
      <Card title={<span className="sec-title"><Icon name="chart" size={15} />Month by month</span>}>
        <div className="months" role="img" aria-label="Distance by month">
          {y.by_month.map(m => (
            <div key={m.month} className="mcol" title={`${MONTHS[m.month - 1]}: ${fmtDist(m.distance_m, u)}, ${m.outings} outings`}>
              <span className="mval tiny">{m.distance_m ? Math.round(m.distance_m / du.f) : ''}</span>
              <span className="mbar"><span style={{ height: `${(m.distance_m / maxD) * 100}%` }} /></span>
              <span className="mlbl tiny muted">{MONTHS[m.month - 1].slice(0, 1)}<span className="hide-sm">{MONTHS[m.month - 1].slice(1)}</span></span>
            </div>
          ))}
        </div>
        <p className="muted tiny m0">In {du.label}.</p>
      </Card>
      <div className="two-grid">
        <Card title={<span className="sec-title"><Icon name="activity" size={15} />By activity</span>} flush>
          <ul className="list">{Object.entries(y.by_activity).sort((a, b) => b[1].distance_m - a[1].distance_m).map(([a, t]) => (
            <li key={a} className="list-row">
              <span className="list-ico"><Icon name={ACTIVITY[a as Activity].icon} size={15} /></span>
              <span className="row-main grow"><span className="row-title">{ACTIVITY[a as Activity].label}</span><span className="muted small">{t.outings} · {fmtMinutes(t.minutes)}</span></span>
              <span className="row-side"><span className="small tnum">{fmtDist(t.distance_m, u)}</span><span className="muted tiny tnum">{fmtHeight(t.gain_m, u)} up</span></span>
            </li>))}</ul>
          {!Object.keys(y.by_activity).length && <p className="muted pad">Finish an outing and it counts here.</p>}
        </Card>
        {y.longest && <Card className="side-card">
          <h4 className="side-h">The longest</h4>
          <Link to={`/outings/${y.longest.id}`} className="link-btn"><Icon name={ACTIVITY[y.longest.activity].icon} size={14} />{outingTitle(y.longest)}</Link>
          <span className="muted small">{fmtDate(y.longest.date)} · {fmtDist(y.longest.actual?.distance_m || y.longest.distance_m, u)}</span>
        </Card>}
      </div>
    </div>
  )
}
