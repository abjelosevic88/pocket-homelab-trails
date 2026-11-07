// One route: the map with the spots along it, the height profile, the
// numbers, how it went the times you did it, and your saved places nearby.
import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { ACTIVITY, WAYPOINT, fmtDist, fmtHeight, fmtMinutes } from '../lib/trails'
import type { NearPlace, Outing, Route, Waypoint } from '../lib/types'
import { openCreate, useCrumb } from '../components/Layout'
import { LevelPill, OutingRow } from '../components/Bits'
import { AppOff } from '../components/Apps'
import { Elevation } from '../components/Elevation'
import { RouteForm, WaypointForm } from '../components/Forms'
import { TrailMap } from '../components/TrailMap'
import { Icon } from '../components/icons'
import { Card, Confirm, Loading, MenuButton, errMsg, useAsync, useToast } from '../components/ui'

export default function RoutePage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const appState = useAppState()
  const places = appState('places', 'places:read')
  const { data, setData, reload, error } = useAsync(() => api.get<{ route: Route; outings: Outing[] }>(`${V1}/routes/${id}`), [id])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const { data: near } = useAsync(() => places.ok ? api.get<{ places: NearPlace[]; open_url?: string; off?: string }>(`${V1}/routes/${id}/places`).catch(() => null) : Promise.resolve(null), [id, places.ok])
  const [hover, setHover] = useState<number | null>(null)
  const [editing, setEditing] = useState(false)
  const [picking, setPicking] = useState(false)
  const [spot, setSpot] = useState<{ wp?: Waypoint; at?: [number, number] } | null>(null)
  const [del, setDel] = useState(false)
  useCrumb(data?.route.name)
  if (error) return <div className="page"><p className="warn-text">{error}</p><Link to="/routes">Back to your routes</Link></div>
  if (!data) return <Loading />
  const r = data.route
  const done = data.outings.filter(o => o.status === 'done')
  const planned = data.outings.filter(o => o.status === 'planned' || o.status === 'out')
  const conditions = done.filter(o => o.conditions).slice(0, 3)
  const put = async (body: Partial<Route>) => {
    try { const p = await api.put<{ route: Route; outings: Outing[] }>(`${V1}/routes/${r.id}`, body); setData(p); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const reverse = async () => { try { setData(await api.post(`${V1}/routes/${r.id}/reverse`)); toast('It now runs the other way round', 'ok') } catch (e) { toast(errMsg(e), 'err') } }
  const remove = async () => { try { await api.del(`${V1}/routes/${r.id}`); announceChange(); navigate('/routes') } catch (e) { toast(errMsg(e), 'err') } }
  const directions = r.start_lat != null ? `https://www.google.com/maps/dir/?api=1&destination=${r.start_lat},${r.start_lng}` : ''
  const menu = () => [
    { label: 'Change the details', icon: 'edit', onSelect: () => setEditing(true) },
    { label: 'Change the way on the map', icon: 'pen', onSelect: () => navigate(`/routes/${r.id}/draw`) },
    { label: 'Walk it the other way round', icon: 'repeat', onSelect: reverse },
    ...(directions ? [{ label: 'Directions to the start', icon: 'navigation', onSelect: () => window.open(directions, '_blank') }] : []),
    { label: 'Print the route sheet', icon: 'file', onSelect: () => window.print() },
    'sep' as const,
    { label: r.archived ? 'Bring it back' : 'Put it away', icon: 'archive', onSelect: () => put({ archived: !r.archived }) },
    { label: 'Delete the route', icon: 'trash', danger: true, onSelect: () => setDel(true) },
  ]
  return (
    <div className="det-page route-page">
      <div className="det-head">
        <div className="grow">
          <div className="det-kicker"><span className="muted small inline-ico"><Icon name={ACTIVITY[r.activity].icon} size={14} />{ACTIVITY[r.activity].label}</span><LevelPill level={r.level} />{r.loop && <span className="pill">Loop</span>}{r.archived && <span className="pill st-paused">Put away</span>}</div>
          <h1 className="det-title">{r.name}</h1>
          <p className="muted m0">{[r.region, r.start_name && `from ${r.start_name}`].filter(Boolean).join(' · ')}</p>
        </div>
        <div className="row det-tools">
          <button type="button" className={`btn ghost icon-only ${r.favourite ? 'fav-on' : ''}`} aria-pressed={r.favourite} aria-label={r.favourite ? 'A favourite' : 'Make it a favourite'} title={r.favourite ? 'A favourite' : 'Make it a favourite'} onClick={() => put({ favourite: !r.favourite })}><Icon name="star" size={17} /></button>
          <a className="btn" href={`${V1}/routes/${r.id}/gpx`} download title="For OsmAnd, Organic Maps, Komoot or a watch: the way to have it offline"><Icon name="download" size={15} /><span className="hide-sm">GPX</span></a>
          <button type="button" className="btn primary" onClick={() => openCreate('outing', { route_id: r.id })}><Icon name="calendar-plus" size={15} />Plan an outing</button>
          <MenuButton label="More" align="right" width={250} items={menu}><Icon name="more" size={17} /></MenuButton>
        </div>
      </div>

      <div className="stats-row">
        <div className="stat"><span className="stat-l">Distance</span><span className="stat-v">{fmtDist(r.distance_m, u)}</span></div>
        <div className="stat"><span className="stat-l">Up</span><span className="stat-v">{fmtHeight(r.gain_m, u)}</span></div>
        <div className="stat"><span className="stat-l">Down</span><span className="stat-v">{fmtHeight(r.loss_m, u)}</span></div>
        <div className="stat"><span className="stat-l">Time</span><span className="stat-v">{fmtMinutes(r.minutes)}</span>{r.minutes_set > 0 && <span className="stat-s tiny">your own</span>}</div>
        <div className="stat"><span className="stat-l">Highest</span><span className="stat-v">{fmtHeight(r.max_ele, u)}</span></div>
      </div>

      <div className="route-map-card card">
        <TrailMap track={r.track} waypoints={r.waypoints} places={near?.places} activity={r.activity} hoverAt={hover} className="route-map" fitKey={`r${r.id}`}
          pickWaypoint={picking} onPick={(lat, lng) => { setPicking(false); setSpot({ at: [lat, lng] }) }} onWaypoint={w => setSpot({ wp: w })} />
        {picking && <div className="map-hint"><Icon name="pin" size={14} />Tap the map where it is<button type="button" className="btn sm" onClick={() => setPicking(false)}>Cancel</button></div>}
        {r.track && <Elevation track={r.track} waypoints={r.waypoints} onHover={setHover} />}
      </div>

      <div className="det-grid">
        <div className="stack">
          <Card title={<span className="sec-title"><Icon name="pin" size={15} />Along the way</span>} actions={<button type="button" className="btn ghost sm" onClick={() => setPicking(true)}><Icon name="plus" size={14} />Add a spot</button>} flush>
            {!r.waypoints?.length ? <p className="muted pad">Mark water, huts, the summit, the car park or a tricky bit: tap “Add a spot”, then the map.</p> : (
              <ul className="list">{r.waypoints.map(w => (
                <li key={w.id}><button type="button" className="list-row linkrow" onClick={() => setSpot({ wp: w })}>
                  <span className={`list-ico wp-ico k-${w.kind}`}><Icon name={(WAYPOINT[w.kind] || WAYPOINT.other).icon} size={15} /></span>
                  <span className="row-main grow"><span className="row-title">{w.name || WAYPOINT[w.kind].label}</span>{w.note && <span className="muted small ellipsis">{w.note}</span>}</span>
                  <span className="row-side"><span className="small tnum">{fmtDist(w.at_m, u)}</span>{w.ele != null && <span className="muted tiny tnum">{fmtHeight(w.ele, u)}</span>}</span>
                </button></li>))}</ul>
            )}
          </Card>
          {(planned.length > 0 || done.length > 0) && <Card title={<span className="sec-title"><Icon name="calendar" size={15} />Your outings here</span>} flush>
            <ul className="list">{[...planned, ...done].map(o => <li key={o.id}><OutingRow o={o} /></li>)}</ul>
          </Card>}
        </div>
        <div className="stack">
          {(r.description || r.tags.length > 0 || r.link_url) && <Card className="side-card">
            <h4 className="side-h">About it</h4>
            {r.description && <p className="pre m0">{r.description}</p>}
            {r.tags.length > 0 && <div className="chips">{r.tags.map(t => <span key={t} className="chip">{t}</span>)}</div>}
            {r.link_url && <a href={r.link_url} target="_blank" rel="noreferrer" className="link-btn small"><Icon name="external" size={13} />Where it came from</a>}
          </Card>}
          {conditions.length > 0 && <Card className="side-card">
            <h4 className="side-h">Trail conditions you noted</h4>
            <ul className="mini-list">{conditions.map(o => <li key={o.id}><Link to={`/outings/${o.id}`}><Icon name="journal" size={12} />{fmtDate(o.date)}</Link><span className="muted">{o.conditions}</span></li>)}</ul>
          </Card>}
          <Card className="side-card">
            <h4 className="side-h">Your places nearby</h4>
            {places.ok ? (!near ? <span className="muted small">Looking…</span> : near.places.length === 0 ? <span className="muted small">None of your Pocket Places within 1.5 km of the way.</span> : (
              <ul className="mini-list">{near.places.map(p => <li key={p.id}><a href={p.url} target="_blank" rel="noreferrer"><Icon name="pin" size={12} />{p.name}</a><span className="muted tiny">at {fmtDist(p.at_m, u)}{p.off_m > 150 ? `, ${fmtDist(p.off_m, u)} off the way` : ''}{p.status === 'want' ? ' · want to go' : ''}</span></li>)}</ul>
            )) : <AppOff slug="places" app={places.app} compact>Huts and restaurants you saved show here.</AppOff>}
          </Card>
          <Card className="side-card">
            <h4 className="side-h">Offline on the trail</h4>
            <p className="muted small m0">Download the GPX and open it in a map app that keeps maps offline (OsmAnd, Organic Maps), or print the route sheet.</p>
            <div className="row wrap"><a className="btn sm" href={`${V1}/routes/${r.id}/gpx`} download><Icon name="download" size={13} />GPX</a><button type="button" className="btn sm" onClick={() => window.print()}><Icon name="file" size={13} />Print</button></div>
          </Card>
        </div>
      </div>
      {editing && <RouteForm route={r} onClose={() => setEditing(false)} onSaved={() => reload()} />}
      {spot && <WaypointForm routeId={r.id} waypoint={spot.wp} at={spot.at} onClose={() => setSpot(null)} onSaved={() => reload()} />}
      {del && <Confirm title="Delete the route?" message={data.outings.length ? `Your ${data.outings.length} outings on it stay, without the route.` : 'It cannot be brought back.'} confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={remove} />}
    </div>
  )
}
