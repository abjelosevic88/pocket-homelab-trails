// Outings: what is coming, and everything done (with how it went), by month.
import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { todayISO } from '../lib/format'
import { fmtDist, fmtHeight } from '../lib/trails'
import type { Outing } from '../lib/types'
import { openCreate } from '../components/Layout'
import { OutingRow } from '../components/Bits'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, Segmented, useAsync, useDebounce } from '../components/ui'

type Show = 'coming' | 'done' | 'off'

export default function Outings() {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const [sp, setSp] = useSearchParams()
  const show = (sp.get('show') as Show) || 'coming'
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const t = todayISO()
  const { data, reload } = useAsync(() => {
    const p = new URLSearchParams()
    if (show === 'coming') { p.set('status', 'planned,out') } else if (show === 'done') { p.set('status', 'done'); p.set('order', 'desc') } else { p.set('status', 'cancelled'); p.set('order', 'desc') }
    if (dq.trim()) p.set('q', dq.trim())
    return api.get<Outing[]>(`${V1}/outings?${p}`)
  }, [show, dq])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const groups = useMemo(() => {
    const m = new Map<string, Outing[]>()
    for (const o of data || []) {
      const k = show === 'coming' ? (o.status === 'out' ? 'Out now' : o.date < t ? 'Not marked yet' : 'Coming up') : new Date(o.date + 'T12:00').toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
      m.set(k, [...(m.get(k) || []), o])
    }
    return [...m.entries()]
  }, [data, show, t])
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Outings</h1></div>
        <button type="button" className="btn primary" onClick={() => openCreate('outing')}><Icon name="calendar-plus" size={15} />Plan an outing</button>
      </div>
      <div className="row wrap filters">
        <Segmented className="self-start" value={show} onChange={v => setSp(v === 'coming' ? {} : { show: v })} options={[{ value: 'coming', label: 'Coming' }, { value: 'done', label: 'Done' }, { value: 'off', label: 'Called off' }]} />
        <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find an outing, a person, a note" aria-label="Find an outing" /></div>
      </div>
      {!data ? <Loading /> : !data.length ? (
        <Empty title={show === 'coming' ? 'Nothing planned' : show === 'done' ? 'Nothing done yet' : 'Nothing called off'} icon="calendar">
          {show === 'coming' && <button type="button" className="btn" onClick={() => openCreate('outing')}><Icon name="calendar-plus" size={15} />Plan an outing</button>}
        </Empty>
      ) : groups.map(([title, list]) => {
        const dist = list.reduce((s, o) => s + (o.actual?.distance_m || o.distance_m), 0), gain = list.reduce((s, o) => s + (o.actual?.gain_m || o.gain_m), 0)
        return (
          <Card key={title} title={<span className="sec-title">{title}<span className="count">{list.length}</span></span>} actions={show === 'done' ? <span className="muted small">{fmtDist(dist, u)} · {fmtHeight(gain, u)} up</span> : undefined} flush>
            <ul className="list">{list.map(o => <li key={o.id}><OutingRow o={o} /></li>)}</ul>
          </Card>
        )
      })}
    </div>
  )
}
