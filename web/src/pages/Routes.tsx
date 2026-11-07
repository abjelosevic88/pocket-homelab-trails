// Your routes: find one by name, what it is, or how long and how hard.
import { useCallback, useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { ACTIVITIES, ACTIVITY } from '../lib/trails'
import type { Route } from '../lib/types'
import { openCreate } from '../components/Layout'
import { RouteRow } from '../components/Bits'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Empty, Loading, MenuButton, Segmented, useAsync, useDebounce } from '../components/ui'

type Sort = 'recent' | 'name' | 'short' | 'long' | 'climb'

export default function Routes() {
  const { settings } = useApp()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 200)
  const [activity, setActivity] = useState('')
  const [level, setLevel] = useState('')
  const [sort, setSort] = useState<Sort>('recent')
  const [archived, setArchived] = useState(false)
  const { data, reload } = useAsync(() => api.get<Route[]>(`${V1}/routes?archived=${archived}`), [archived])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const list = useMemo(() => {
    let l = (data || []).filter(r => (!activity || r.activity === activity) && (!level || r.level === level))
    const t = dq.trim().toLowerCase()
    if (t) l = l.filter(r => `${r.name} ${r.region} ${r.start_name} ${r.tags.join(' ')}`.toLowerCase().includes(t))
    const by: Record<Sort, (a: Route, b: Route) => number> = {
      recent: () => 0, name: (a, b) => a.name.localeCompare(b.name), short: (a, b) => a.distance_m - b.distance_m, long: (a, b) => b.distance_m - a.distance_m, climb: (a, b) => b.gain_m - a.gain_m,
    }
    return [...l].sort(by[sort])
  }, [data, activity, level, dq, sort])
  const add = () => [
    { label: 'Bring in a route (GPX, KML…)', icon: 'upload', onSelect: () => openCreate('import') },
    { label: 'Draw a route on the map', icon: 'pen', onSelect: () => openCreate('draw') },
    { label: 'A route from a day you recorded', icon: 'navigation', onSelect: () => openCreate('movement') },
  ]
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Routes</h1><p className="muted">{data ? `${data.length} ${archived ? 'put away' : data.length === 1 ? 'route' : 'routes'}` : ''}{settings?.pace_pct && settings.pace_pct !== 100 ? ` · times at ${settings.pace_pct}% of the usual speed` : ''}</p></div>
        <MenuButton label="Add a route" className="btn primary" align="right" width={280} items={add}><Icon name="plus" size={15} />Add a route</MenuButton>
      </div>
      <div className="row wrap filters">
        <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find a route" aria-label="Find a route" /></div>
        <Select ariaLabel="Activity" value={activity} onChange={setActivity} options={[{ value: '', label: 'Everything' }, ...ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))]} />
        <Select ariaLabel="How hard" value={level} onChange={setLevel} options={[{ value: '', label: 'Any difficulty' }, { value: 'easy', label: 'Easy' }, { value: 'moderate', label: 'Moderate' }, { value: 'hard', label: 'Hard' }, { value: 'expert', label: 'Very hard' }]} />
        <Select ariaLabel="Order" value={sort} onChange={v => setSort(v as Sort)} options={[{ value: 'recent', label: 'Latest first' }, { value: 'name', label: 'By name' }, { value: 'short', label: 'Shortest first' }, { value: 'long', label: 'Longest first' }, { value: 'climb', label: 'Most climb first' }]} />
        <Segmented size="sm" value={archived ? 'a' : 'c'} onChange={v => setArchived(v === 'a')} options={[{ value: 'c', label: 'Routes' }, { value: 'a', label: 'Put away' }]} />
      </div>
      {!data ? <Loading /> : !list.length ? (
        <Empty title={data.length ? 'No route fits' : archived ? 'Nothing put away' : 'No routes yet'} icon="route">
          {!data.length && !archived && <div className="row wrap center"><button type="button" className="btn" onClick={() => openCreate('import')}><Icon name="upload" size={15} />Bring in a GPX</button><button type="button" className="btn" onClick={() => openCreate('draw')}><Icon name="pen" size={15} />Draw one</button></div>}
        </Empty>
      ) : (
        <Card flush><ul className="list">{list.map(r => <li key={r.id}><RouteRow r={r} /></li>)}</ul></Card>
      )}
    </div>
  )
}
