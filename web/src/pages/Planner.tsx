// Drawing a route: tap the map where you want to go. The way is straight
// lines between the taps, or follows the paths (BRouter); heights come in
// as you draw, with the distance, the climb and the time.
import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange } from '../lib/data'
import { ACTIVITIES, ACTIVITY, LEVEL, fmtDist, fmtHeight, fmtMinutes } from '../lib/trails'
import type { Activity, DrawResult, FoundPlace, Route } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Elevation } from '../components/Elevation'
import { TrailMap } from '../components/TrailMap'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Segmented, Spinner, errMsg, useDebounce, useToast } from '../components/ui'

type C = [number, number]

export default function Planner() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const online = settings?.online !== false && !settings?.demo
  const { toast } = useToast()
  const [route, setRoute] = useState<Route | null>(null)
  const [controls, setControls] = useState<C[]>([])
  const [history, setHistory] = useState<C[][]>([])
  const [mode, setMode] = useState<'straight' | 'paths'>(online ? 'paths' : 'straight')
  const [activity, setActivity] = useState<Activity>(settings?.default_activity || 'hike')
  const [name, setName] = useState('')
  const [res, setRes] = useState<DrawResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')
  const [q, setQ] = useState('')
  const [found, setFound] = useState<FoundPlace[] | null>(null)
  const [view, setView] = useState<{ lat: number; lng: number; zoom: number; key: string } | null>(null)
  const [hover, setHover] = useState<number | null>(null)
  const [panel, setPanel] = useState(() => window.innerWidth > 767)
  useCrumb(route ? route.name : 'Draw a route')

  // an existing route: its drawn points, or its track thinned to points
  useEffect(() => {
    if (!id) return
    api.get<{ route: Route }>(`${V1}/routes/${id}`).then(({ route: r }) => {
      setRoute(r); setName(r.name); setActivity(r.activity)
      if (r.controls?.length) { setControls(r.controls); setMode(r.drawn === 'paths' && online ? 'paths' : 'straight') }
      else if (r.track?.length) {
        const t = r.track, step = Math.max(1, Math.ceil(t.length / 40))
        const pts: C[] = t.filter((_, i) => i % step === 0 || i === t.length - 1).map(p => [p[0], p[1]])
        setControls(pts); setMode('straight')
      }
    }).catch(e => toast(errMsg(e), 'err'))
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps

  const change = (c: C[]) => { setHistory(h => [...h.slice(-40), controls]); setControls(c) }
  const undo = () => setHistory(h => { if (!h.length) return h; setControls(h[h.length - 1]); return h.slice(0, -1) })

  // the way and its numbers, a moment after the last tap
  const key = JSON.stringify([controls, mode, activity])
  const dkey = useDebounce(key, 500)
  const seq = useRef(0)
  useEffect(() => {
    if (controls.length < 2) { setRes(null); setErr(''); return }
    const n = ++seq.current
    setBusy(true)
    api.post<DrawResult>(`${V1}/routes/draw`, { controls, mode, activity })
      .then(r => { if (n === seq.current) { setRes(r); setErr('') } })
      .catch(e => { if (n === seq.current) setErr(errMsg(e)) })
      .finally(() => { if (n === seq.current) setBusy(false) })
  }, [dkey]) // eslint-disable-line react-hooks/exhaustive-deps

  const search = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!q.trim()) return
    try { setFound(await api.get<FoundPlace[]>(`${V1}/lookup/places?q=${encodeURIComponent(q.trim())}`)) } catch (e2) { toast(errMsg(e2), 'err') }
  }
  const save = async () => {
    if (!res || controls.length < 2) return
    setSaving(true)
    try {
      const body = { name: name.trim() || `${ACTIVITY[activity].label} ${new Date().toLocaleDateString()}`, activity, track: res.track, controls, drawn: mode }
      if (route) {
        await api.put(`${V1}/routes/${route.id}`, body)
        announceChange(); navigate(`/routes/${route.id}`)
      } else {
        const r = await api.post<Route>(`${V1}/routes`, body)
        announceChange(); navigate(`/routes/${r.id}`)
      }
    } catch (e) { toast(errMsg(e), 'err') } finally { setSaving(false) }
  }
  const loopBack = () => { if (controls.length >= 2) change([...controls, controls[0]]) }
  const reverse = () => change([...controls].reverse())

  return (
    <div className="planner">
      <TrailMap draw={controls} onDraw={change} track={res?.track} activity={activity} hoverAt={hover} className="planner-map" fitKey={route ? `r${route.id}` : 'new'} view={view} />
      <div className={`planner-panel ${panel ? '' : 'folded'}`}>
        <button type="button" className="planner-fold" onClick={() => setPanel(p => !p)} aria-expanded={panel}>
          <span className="grow ellipsis"><b>{res ? `${fmtDist(res.distance_m, u)} · ${fmtHeight(res.gain_m, u)} up · ${fmtMinutes(res.minutes)}` : controls.length ? 'Tap the next point' : 'Tap the map to start'}</b></span>
          {busy && <Spinner />}
          <Icon name={panel ? 'chevron-down' : 'chevron-up'} size={16} />
        </button>
        {panel && <div className="planner-body stack">
          <form className="planner-search" onSubmit={search}>
            <input value={q} onChange={e => { setQ(e.target.value); if (!e.target.value) setFound(null) }} placeholder={online ? 'Go to a mountain, a village, a car park…' : 'Place search is switched off'} aria-label="Find a place" disabled={!online} />
            <button type="submit" className="btn icon-only" aria-label="Find" disabled={!online || !q.trim()}><Icon name="search" size={15} /></button>
          </form>
          {found && <div className="pick-list">{found.length === 0 ? <p className="muted small pad-line">Nothing found.</p> : found.map((p, i) => (
            <button key={i} type="button" className="pick-row" onClick={() => { setView({ lat: p.lat, lng: p.lng, zoom: 14, key: `${p.lat},${p.lng},${Date.now()}` }); setFound(null) }}>
              <Icon name="pin" size={14} /><span className="grow ellipsis"><b>{p.name}</b><span className="muted"> · {p.detail}</span></span>{p.ele != null && <span className="muted small">{p.ele} m</span>}
            </button>))}</div>}
          <div className="planner-mode">
            <Select ariaLabel="Activity" value={activity} onChange={v => setActivity(v as Activity)} options={ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))} />
            <Segmented size="sm" value={mode} onChange={v => setMode(v as 'straight' | 'paths')} label="How the way goes"
              options={[{ value: 'paths', label: 'Along paths', disabled: !online, title: online ? 'The way follows trails and roads (BRouter)' : 'Needs looking things up online (Settings → General)' }, { value: 'straight', label: 'Straight lines' }]} />
          </div>
          <div className="planner-tools">
            <button type="button" className="btn sm" onClick={undo} disabled={!history.length}><Icon name="undo" size={14} />Undo</button>
            <button type="button" className="btn sm" onClick={loopBack} disabled={controls.length < 2} title="Back to where you started"><Icon name="repeat" size={14} />Back to start</button>
            <button type="button" className="btn sm" onClick={reverse} disabled={controls.length < 2}><Icon name="arrow-up" size={14} />Reverse</button>
            <button type="button" className="btn ghost sm" onClick={() => change([])} disabled={!controls.length}><Icon name="trash" size={14} />Clear</button>
          </div>
          <p className="muted tiny m0">Tap to add a point · drag a point to move it · tap a point to remove it.</p>
          {err && <p className="warn-text small m0">{err}</p>}
          {res && <>
            <div className="planner-stats">
              <div className="stat"><span className="stat-l">Distance</span><span className="stat-v">{fmtDist(res.distance_m, u)}</span></div>
              <div className="stat"><span className="stat-l">Up</span><span className="stat-v">{fmtHeight(res.gain_m, u)}</span></div>
              <div className="stat"><span className="stat-l">Time</span><span className="stat-v">{fmtMinutes(res.minutes)}</span><span className="stat-s tiny">{LEVEL[res.level]?.label.toLowerCase()}</span></div>
            </div>
            {res.heights_note && <p className="muted tiny m0">{res.heights_note}</p>}
            {res.track.some(p => p[2] != null) && <Elevation track={res.track} onHover={setHover} compact />}
          </>}
          <div className="planner-save">
            <input value={name} onChange={e => setName(e.target.value)} placeholder="Name the route" aria-label="Name" />
            <button type="button" className="btn primary" onClick={save} disabled={saving || !res || busy}>{saving ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
          </div>
          {route && !route.controls?.length && <p className="muted tiny m0">This route came from a file; it was thinned to {controls.length} points to change it. Saving replaces its track.</p>}
        </div>}
      </div>
    </div>
  )
}
