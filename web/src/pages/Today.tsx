// Today: out now (with "I'm back"), the next outing with its weather and
// what to know, outings that happened but were never marked, how the
// recent ones went, routes not done yet, and the year so far.
import { useCallback, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { ACTIVITY, fmtDist, fmtHeight, fmtMinutes, outingTitle } from '../lib/trails'
import type { OutingPage, TodayView } from '../lib/types'
import { openCreate } from '../components/Layout'
import { AdviceList, OutingRow, PackedBar, RouteRow, WeatherLine } from '../components/Bits'
import { Icon } from '../components/icons'
import { Card, Loading, Spinner, errMsg, useAsync, useToast } from '../components/ui'

export default function Today() {
  const { user, settings } = useApp()
  const u = settings?.units || 'metric'
  const { data, reload } = useAsync(() => api.get<TodayView>(`${V1}/today`), [])
  useOnChange(useCallback(() => { reload() }, [reload]))
  if (!data) return <Loading />
  if (!data.routes && !data.current && !data.recent.length) return <Welcome name={user?.name} />
  const hello = (() => { const h = new Date().getHours(); return h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening' })()
  const y = data.year
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{hello}{user?.name ? `, ${user.name.split(' ')[0]}` : ''}</h1>
          <p className="muted">{data.current?.outing.status === 'out' ? 'You are out now.' : data.current ? `Next: ${outingTitle(data.current.outing)}, ${dayWords(data.current.outing.date, data.today)}.` : 'Nothing planned yet.'}</p></div>
      </div>
      {data.current && <NextCard page={data.current} today={data.today} onChange={reload} />}
      {data.unmarked.length > 0 && <Unmarked list={data.unmarked} onChange={reload} />}
      <div className="tiles">
        <Link to="/year" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="calendar-check" size={14} /></span>Outings in {y.year}</div><div className="tile-val">{y.outings}</div></Link>
        <Link to="/year" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="route" size={14} /></span>Distance</div><div className="tile-val small-val">{fmtDist(y.distance_m, u)}</div></Link>
        <Link to="/year" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="mountain" size={14} /></span>Climbed</div><div className="tile-val small-val">{fmtHeight(y.gain_m, u)}</div></Link>
        <Link to="/year" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="clock" size={14} /></span>Time out</div><div className="tile-val small-val">{fmtMinutes(y.minutes)}</div></Link>
      </div>
      <div className="two-grid">
        {data.upcoming.length > 0 && <Card title={<span className="sec-title"><Icon name="calendar" size={15} />Coming up</span>} actions={<Link to="/outings" className="btn ghost sm">All</Link>} flush>
          <ul className="list">{data.upcoming.map(o => <li key={o.id}><OutingRow o={o} right={<span className="muted small">{dayWords(o.date, data.today)}</span>} /></li>)}</ul>
        </Card>}
        {data.recent.length > 0 && <Card title={<span className="sec-title"><Icon name="check-circle" size={15} />Lately</span>} actions={<Link to="/outings?show=done" className="btn ghost sm">All</Link>} flush>
          <ul className="list">{data.recent.map(o => <li key={o.id}><OutingRow o={o} /></li>)}</ul>
        </Card>}
        {data.ideas.length > 0 && <Card title={<span className="sec-title"><Icon name="flag" size={15} />Not done yet</span>} actions={<Link to="/routes" className="btn ghost sm">Routes</Link>} flush>
          <ul className="list">{data.ideas.map(r => <li key={r.id}><RouteRow r={r} extra={<button type="button" className="btn sm" onClick={e => { e.preventDefault(); openCreate('outing', { route_id: r.id }) }}>Plan</button>} /></li>)}</ul>
        </Card>}
      </div>
    </div>
  )
}

function dayWords(d: string, today: string): string {
  const a = new Date(today + 'T12:00'), b = new Date(d + 'T12:00')
  const n = Math.round((b.getTime() - a.getTime()) / 86400000)
  if (n === 0) return 'today'
  if (n === 1) return 'tomorrow'
  if (n === -1) return 'yesterday'
  if (n > 1 && n < 7) return b.toLocaleDateString(undefined, { weekday: 'long' })
  if (n >= 7 && n < 60) return `in ${n} days`
  return fmtDate(d)
}

/** The next outing (or the one you are on) with what to know. */
function NextCard({ page, today, onChange }: { page: OutingPage; today: string; onChange: () => void }) {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const [busy, setBusy] = useState('')
  const o = page.outing, r = page.route
  const out = o.status === 'out'
  const act = async (a: 'start' | 'back') => {
    setBusy(a)
    try {
      const p = await api.post<OutingPage>(`${V1}/outings/${o.id}/status/${a}`)
      toast(a === 'back' ? (p.note || 'Welcome back') : 'Have a good one', 'ok')
      announceChange(); onChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  return (
    <section className={`next-card ${out ? 'is-out' : ''}`}>
      <div className="next-top">
        <span className="next-ico"><Icon name={out ? 'navigation' : ACTIVITY[o.activity].icon} size={20} /></span>
        <div className="grow min0">
          <div className="next-kicker">{out ? 'Out now' : dayWords(o.date, today)}{o.start_time && !out ? ` · ${o.start_time}` : ''}{o.back_by ? ` · back by ${o.back_by.slice(11, 16)}` : ''}</div>
          <Link to={`/outings/${o.id}`} className="next-title">{outingTitle(o)}</Link>
          {r && <div className="muted small">{fmtDist(r.distance_m, u)} · {fmtHeight(r.gain_m, u)} up · about {fmtMinutes(r.minutes)}{page.finish ? ` · finish around ${page.finish}` : ''}</div>}
        </div>
        <div className="next-act">
          {out ? <button type="button" className="btn primary" onClick={() => act('back')} disabled={!!busy}>{busy === 'back' ? <Spinner /> : <Icon name="home" size={15} />}I'm back</button>
            : o.date === today && <button type="button" className="btn primary" onClick={() => act('start')} disabled={!!busy}>{busy === 'start' ? <Spinner /> : <Icon name="play" size={15} />}Setting off</button>}
        </div>
      </div>
      <WeatherLine w={o.weather} sunset={page.sunset} />
      {page.weather_note && !o.weather && <p className="muted small m0">{page.weather_note}</p>}
      <AdviceList items={page.advice} />
      {o.item_count > 0 && <Link to={`/outings/${o.id}#pack`} className="next-pack"><Icon name="backpack" size={14} /><span>{o.packed_count} of {o.item_count} packed</span><PackedBar packed={o.packed_count} total={o.item_count} /></Link>}
      {o.safety && o.contact_name && <p className="muted small m0"><Icon name="shield" size={12} className="inline-ico" /> {o.contact_name} is told if you are not back by {o.back_by.slice(11, 16)}{o.alert_after ? ` (after ${fmtMinutes(o.alert_after)})` : ''}.</p>}
    </section>
  )
}

/** Planned for a day that has passed: did it happen? */
function Unmarked({ list, onChange }: { list: TodayView['unmarked']; onChange: () => void }) {
  const { toast } = useToast()
  const [busy, setBusy] = useState(0)
  const mark = async (id: number, a: 'done' | 'cancel') => {
    setBusy(id)
    try { await api.post(`${V1}/outings/${id}/status/${a}`); announceChange(); onChange() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(0) }
  }
  return (
    <Card title={<span className="sec-title"><Icon name="help" size={15} />Did these happen?</span>} flush>
      <ul className="list">{list.map(o => (
        <li key={o.id} className="list-row">
          <span className="list-ico st-due"><Icon name={ACTIVITY[o.activity].icon} size={15} /></span>
          <Link to={`/outings/${o.id}`} className="row-main grow"><span className="row-title">{outingTitle(o)}</span><span className="muted small">{fmtDate(o.date)}</span></Link>
          <button type="button" className="btn sm" disabled={busy === o.id} onClick={() => mark(o.id, 'done')}><Icon name="check" size={14} /><span className="hide-sm">It happened</span></button>
          <button type="button" className="btn ghost sm" disabled={busy === o.id} onClick={() => mark(o.id, 'cancel')}><span className="hide-sm">Called off</span><Icon name="x" size={14} className="show-sm" /></button>
        </li>))}</ul>
    </Card>
  )
}

function Welcome({ name }: { name?: string }) {
  return (
    <div className="page stack loose welcome">
      <div><h1 className="page-title">Welcome{name ? `, ${name.split(' ')[0]}` : ''}</h1><p className="muted">Start with a route: a hike, a walk, a run or a ride.</p></div>
      <div className="welcome-steps">
        <button type="button" className="tile" onClick={() => openCreate('import')}><div className="tile-head"><span className="tile-ico"><Icon name="upload" size={14} /></span>Bring in a route</div><p className="muted small m0">A GPX from Komoot, AllTrails, Strava or a watch; a KML from Google My Maps.</p></button>
        <Link to="/routes/draw" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="pen" size={14} /></span>Draw one on the map</div><p className="muted small m0">Tap where you want to go; the way can follow the paths.</p></Link>
        <button type="button" className="tile" onClick={() => openCreate('movement')}><div className="tile-head"><span className="tile-ico"><Icon name="navigation" size={14} /></span>From a day you recorded</div><p className="muted small m0">A walk Pocket Movement recorded becomes a route to do again.</p></button>
        <button type="button" className="tile" onClick={() => openCreate('gear')}><div className="tile-head"><span className="tile-ico"><Icon name="backpack" size={14} /></span>Add your gear</div><p className="muted small m0">What you take and what it weighs, for packing lists.</p></button>
      </div>
    </div>
  )
}
