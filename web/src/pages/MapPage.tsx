// All your routes on one map; tap one to see it, plan it or open it.
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { ACTIVITIES, ACTIVITY, fmtDist, fmtHeight, fmtMinutes } from '../lib/trails'
import type { Route } from '../lib/types'
import { openCreate } from '../components/Layout'
import { LevelPill } from '../components/Bits'
import { TrailMap, type MapLine } from '../components/TrailMap'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { useAsync } from '../components/ui'

export default function MapPage() {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { data } = useAsync(() => api.get<Route[]>(`${V1}/routes`), [])
  const [activity, setActivity] = useState('')
  const [sel, setSel] = useState<number | null>(null)
  const list = useMemo(() => (data || []).filter(r => !activity || r.activity === activity), [data, activity])
  const lines: MapLine[] = useMemo(() => list.map(r => ({ id: r.id, line: r.preview || [], activity: r.activity, name: r.name })), [list])
  const r = list.find(x => x.id === sel)
  return (
    <div className="map-page">
      <TrailMap lines={lines} selected={sel} onSelect={setSel} activity={(activity || 'hike') as Route['activity']} className="map-full" fitKey={activity} />
      <div className="map-filter"><Select ariaLabel="Activity" value={activity} onChange={v => { setActivity(v); setSel(null) }} options={[{ value: '', label: `Everything (${data?.length ?? 0})` }, ...ACTIVITIES.filter(a => data?.some(x => x.activity === a)).map(a => ({ value: a, label: ACTIVITY[a].label }))]} /></div>
      {r && <div className="map-card">
        <div className="row tight"><Icon name={ACTIVITY[r.activity].icon} size={16} /><Link to={`/routes/${r.id}`} className="grow ellipsis map-card-title">{r.name}</Link><button type="button" className="btn ghost icon-only sm" aria-label="Close" onClick={() => setSel(null)}><Icon name="x" size={15} /></button></div>
        <div className="muted small">{fmtDist(r.distance_m, u)} · {fmtHeight(r.gain_m, u)} up · about {fmtMinutes(r.minutes)}{r.region ? ` · ${r.region}` : ''}</div>
        <div className="row tight"><LevelPill level={r.level} />{r.done > 0 && <span className="muted tiny">done {r.done}×</span>}<span className="spacer" />
          <Link to={`/routes/${r.id}`} className="btn sm">Open</Link><button type="button" className="btn primary sm" onClick={() => openCreate('outing', { route_id: r.id })}>Plan</button></div>
      </div>}
    </div>
  )
}
