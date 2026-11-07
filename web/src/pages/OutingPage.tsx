// One outing: before (the weather, what to know, what to pack, who to tell),
// during ("I'm back"), and after (how it went, the walked track against the
// plan, into Pocket Health and Pocket Notes).
import { useCallback, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState, useOnChange } from '../lib/data'
import { fmtDate, fmtDateTime, todayISO } from '../lib/format'
import { ACTIVITY, GEAR, GEAR_KINDS, STATUS, fmtDist, fmtHeight, fmtMinutes, fmtSeconds, fmtTemp, fmtWeight, outingTitle, weatherIcon } from '../lib/trails'
import type { Gear, GearKind, OutingItem, OutingPage as Page, Pack } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { AdviceList, Pill, WeatherLine } from '../components/Bits'
import { AppOff, offTitle } from '../components/Apps'
import { Elevation } from '../components/Elevation'
import { OutingForm, Stars } from '../components/Forms'
import { TrailMap } from '../components/TrailMap'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Confirm, Loading, MenuButton, Modal, Spinner, Switch, errMsg, useAsync, useToast } from '../components/ui'

export default function OutingPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const { data, setData, reload, error } = useAsync(() => api.get<Page>(`${V1}/outings/${id}`), [id])
  useOnChange(useCallback(() => { reload() }, [reload]))
  const [hover, setHover] = useState<number | null>(null)
  const [editing, setEditing] = useState(false)
  const [busy, setBusy] = useState('')
  const [del, setDel] = useState(false)
  useCrumb(data ? outingTitle(data.outing) : undefined)
  if (error) return <div className="page"><p className="warn-text">{error}</p><Link to="/outings">Back to your outings</Link></div>
  if (!data) return <Loading />
  const o = data.outing, r = data.route
  const st = STATUS[o.status]
  const past = o.date < todayISO() || o.status === 'done'
  const act = async (a: string, ok?: string) => {
    setBusy(a)
    try { const p = await api.post<Page>(`${V1}/outings/${o.id}/status/${a}`); setData(p); announceChange(); if (p.note || ok) toast(p.note || ok!, 'ok') } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const remove = async () => { try { await api.del(`${V1}/outings/${o.id}`); announceChange(); navigate('/outings') } catch (e) { toast(errMsg(e), 'err') } }
  const menu = () => [
    { label: 'Change the outing', icon: 'edit', onSelect: () => setEditing(true) },
    ...(o.status === 'planned' ? [{ label: 'It happened (mark it done)', icon: 'check', onSelect: () => act('done') }, { label: 'Call it off', icon: 'x-circle', onSelect: () => act('cancel') }] : []),
    ...(o.status === 'cancelled' || o.status === 'done' ? [{ label: 'Plan it again', icon: 'rotate-ccw', onSelect: () => act('plan') }] : []),
    ...(o.actual_track?.length ? [{ label: 'Download the walked track (GPX)', icon: 'download', onSelect: () => window.open(`${V1}/outings/${o.id}/gpx`) }] : []),
    'sep' as const,
    { label: 'Delete the outing', icon: 'trash', danger: true, onSelect: () => setDel(true) },
  ]
  const actualProfile = o.actual_track && o.actual?.has_ele ? o.actual_track : null
  return (
    <div className="det-page outing-page">
      <div className="det-head">
        <div className="grow">
          <div className="det-kicker"><Pill cls={st.cls} icon={st.icon}>{st.label}</Pill><span className="muted small inline-ico"><Icon name={ACTIVITY[o.activity].icon} size={14} />{ACTIVITY[o.activity].label} · {fmtDate(o.date)}{o.start_time ? `, ${o.start_time}` : ''}</span></div>
          <h1 className="det-title">{outingTitle(o)}</h1>
          {r && <p className="muted m0"><Link to={`/routes/${r.id}`}>{r.name}</Link> · {fmtDist(r.distance_m, u)} · {fmtHeight(r.gain_m, u)} up · about {fmtMinutes(r.minutes)}</p>}
        </div>
        <div className="row det-tools">
          {o.status === 'out' && <button type="button" className="btn primary" onClick={() => act('back', 'Welcome back')} disabled={!!busy}>{busy === 'back' ? <Spinner /> : <Icon name="home" size={15} />}I'm back</button>}
          {o.status === 'planned' && o.date === todayISO() && <button type="button" className="btn primary" onClick={() => act('start', 'Have a good one')} disabled={!!busy}>{busy === 'start' ? <Spinner /> : <Icon name="play" size={15} />}Setting off</button>}
          {o.status === 'planned' && o.date < todayISO() && <button type="button" className="btn primary" onClick={() => act('done')} disabled={!!busy}><Icon name="check" size={15} />It happened</button>}
          <button type="button" className="btn" onClick={() => setEditing(true)}><Icon name="edit" size={15} /><span className="hide-sm">Change</span></button>
          <MenuButton label="More" align="right" width={270} items={menu}><Icon name="more" size={17} /></MenuButton>
        </div>
      </div>

      {o.status === 'out' && <div className={`out-banner ${o.alerted_at ? 'bad' : o.warned_at ? 'warn' : ''}`}>
        <Icon name={o.alerted_at ? 'alert' : 'navigation'} size={18} />
        <div className="grow">
          <b>{o.alerted_at ? `${o.contact_name || o.contact_email} was emailed that you are not back yet.` : o.warned_at ? 'You are past your back-by time.' : 'You are out.'}</b>
          <div className="small">{o.back_by ? `Back by ${o.back_by.slice(11, 16)}` : 'No back-by time set'}{o.safety && o.contact_name && !o.alerted_at ? ` · ${o.contact_name} is told ${o.alert_after ? fmtMinutes(o.alert_after) + ' after that' : 'at that time'}` : ''}{o.alert_error ? ` · ${o.alert_error}` : ''}</div>
        </div>
        <button type="button" className="btn primary" onClick={() => act('back', 'Welcome back')} disabled={!!busy}>I'm back</button>
      </div>}

      {(r?.track || o.actual_track) && <div className="route-map-card card">
        <TrailMap track={r?.track} actual={o.actual_track} waypoints={r?.waypoints} activity={o.activity} hoverAt={actualProfile ? null : hover} className="route-map" fitKey={`o${o.id}`} />
        {(actualProfile || r?.track) && <Elevation track={(actualProfile || r!.track)!} waypoints={actualProfile ? undefined : r?.waypoints} onHover={setHover} />}
        {o.actual_track?.length ? <div className="map-legend"><span><i className="lg-plan" />Planned</span><span><i className="lg-walked" />Walked</span></div> : null}
      </div>}

      <div className={`det-grid ${past ? '' : 'side-first'}`}>
        <div className="stack">
          {past && <HowItWent page={data} onPage={setData} />}
          <Packing page={data} onPage={setData} />
        </div>
        <div className="stack">
          {o.status !== 'done' && o.status !== 'cancelled' && <TheDay page={data} onPage={setData} />}
          <Safety page={data} onPage={setData} />
          {(o.people.length > 0 || o.trip_name || o.notes) && <Card className="side-card">
            {o.people.length > 0 && <><h4 className="side-h">With</h4><div className="chips">{o.people.map((p, i) => <span key={i} className="chip">{p.name || p.email}</span>)}</div></>}
            {o.trip_name && <><h4 className="side-h">Trip</h4>{data.links.travel ? <a href={data.links.travel} target="_blank" rel="noreferrer" className="link-btn"><Icon name="plane" size={13} />{o.trip_name}</a> : <span>{o.trip_name}</span>}</>}
            {o.notes && <><h4 className="side-h">Notes</h4><p className="pre m0">{o.notes}</p></>}
          </Card>}
          <OtherApps page={data} onPage={setData} />
        </div>
      </div>
      {editing && <OutingForm outing={o} onClose={() => setEditing(false)} onSaved={p => setData(p)} />}
      {del && <Confirm title="Delete the outing?" message="Its packing list and walked track go too. The event in Pocket Calendar is removed." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={remove} />}
    </div>
  )
}

// ---- the day: weather, sun, what to know ----

function TheDay({ page, onPage }: { page: Page; onPage: (p: Page) => void }) {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const o = page.outing, w = o.weather
  const refresh = async () => { setBusy(true); try { onPage(await api.post<Page>(`${V1}/outings/${o.id}/weather`)) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  const hours = (w?.hours || []).filter(h => !o.start_time || (h.time >= o.start_time.slice(0, 2) + ':00' && (!page.finish || h.time <= page.finish)))
  return (
    <Card className="side-card day-card">
      <div className="row"><h4 className="side-h grow">The day</h4><button type="button" className="btn ghost icon-only sm" onClick={refresh} disabled={busy} aria-label="Get the forecast again" title="Get the forecast again">{busy ? <Spinner /> : <Icon name="refresh" size={14} />}</button></div>
      {w ? <WeatherLine w={w} sunset={page.sunset} /> : <p className="muted small m0">{page.weather_note || 'The forecast comes about ten days ahead.'}</p>}
      {hours.length > 0 && <div className="hours scroll-x">{hours.map(h => (
        <div key={h.time} className="hour"><span className="tiny muted">{h.time.slice(0, 2)}</span><Icon name={weatherIcon(h.code)} size={15} /><span className="small">{fmtTemp(h.t, u)}</span>{h.precip_prob != null && h.precip_prob >= 20 && <span className="tiny rain">{Math.round(h.precip_prob)}%</span>}</div>
      ))}</div>}
      <dl className="facts">
        {page.sunrise && <><dt>Sunrise</dt><dd>{page.sunrise}</dd></>}
        {page.sunset && <><dt>Sunset</dt><dd>{page.sunset}</dd></>}
        {page.finish && <><dt>You finish</dt><dd>around {page.finish}</dd></>}
      </dl>
      <AdviceList items={page.advice} />
      {w && <p className="muted tiny m0">{w.source === 'weather' ? 'From Pocket Weather' : w.source === 'open-meteo' ? 'From Open-Meteo' : 'Example forecast'}, at the start of the route · {fmtDateTime(w.fetched_at)}</p>}
    </Card>
  )
}

// ---- who to tell ----

function Safety({ page, onPage }: { page: Page; onPage: (p: Page) => void }) {
  const { toast } = useToast()
  const appState = useAppState()
  const mail = appState('mail', 'mail:send')
  const [preview, setPreview] = useState<{ subject: string; body: string; to: string } | null>(null)
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const o = page.outing
  if (o.status === 'done' && !o.alerted_at) return null
  if (o.status === 'cancelled') return null
  const show = async () => { setBusy(true); try { setPreview(await api.post(`${V1}/outings/${o.id}/send-plan`, { preview: true })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) } }
  const send = async () => {
    setBusy(true)
    try { await api.post(`${V1}/outings/${o.id}/send-plan`, { message: msg }); toast(`Sent to ${o.contact_name || o.contact_email}`, 'ok'); setPreview(null); onPage(await api.get<Page>(`${V1}/outings/${o.id}`)) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Card className="side-card">
      <h4 className="side-h">Be back by</h4>
      {o.back_by ? <p className="m0"><b>{o.back_by.slice(11, 16)}</b>{o.back_by.slice(0, 10) !== o.date ? ` on ${fmtDate(o.back_by.slice(0, 10))}` : ''}</p>
        : <p className="muted small m0">No time set.{page.suggest_back_by ? ` About ${page.suggest_back_by.slice(11, 16)} would fit the route.` : ''}</p>}
      {o.safety ? <p className="small m0"><Icon name="shield" size={13} className="inline-ico" /> If you have not tapped “I'm back”, <b>{o.contact_name || o.contact_email}</b> gets an email {o.alert_after ? `${fmtMinutes(o.alert_after)} later` : 'at that time'}.</p>
        : <p className="muted small m0">Nobody is told. Switch it on with “Change”.</p>}
      {o.alerted_at && <p className="warn-text small m0"><Icon name="alert" size={12} className="inline-ico" /> Told at {fmtDateTime(o.alerted_at)}{o.alert_error ? ` — ${o.alert_error}` : ''}.</p>}
      {o.contact_email && o.status !== 'done' && <>
        {mail.ok ? <button type="button" className="btn sm self-start" onClick={show} disabled={busy}>{busy ? <Spinner /> : <Icon name="send" size={13} />}{o.plan_sent_at ? 'Send my plan again' : 'Send my plan now'}</button>
          : <AppOff slug="mail" app={mail.app} compact>Your plan and the “not back yet” email go out through Pocket Mail.</AppOff>}
        {o.plan_sent_at && <span className="muted tiny">Plan sent {fmtDateTime(o.plan_sent_at)}</span>}
      </>}
      {preview && <Modal title={`Your plan to ${preview.to}`} onClose={() => setPreview(null)} footer={<>
        <button type="button" className="btn" onClick={() => setPreview(null)}>Cancel</button>
        <button type="button" className="btn primary" onClick={send} disabled={busy}>{busy ? <Spinner /> : <Icon name="send" size={15} />}Send</button>
      </>}>
        <div className="stack"><p className="muted small m0">{preview.subject}</p><pre className="mail-preview">{preview.body}</pre>
          <textarea rows={2} value={msg} onChange={e => setMsg(e.target.value)} placeholder="Add a line (optional): “Car is the blue Golf at the trailhead.”" /></div>
      </Modal>}
    </Card>
  )
}

// ---- after ----

function HowItWent({ page, onPage }: { page: Page; onPage: (p: Page) => void }) {
  const { settings, setSettings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const appState = useAppState()
  const movement = appState('movement', 'movement:points')
  const movementRead = appState('movement', 'movement:read')
  const o = page.outing
  const [f, setF] = useState({ rating: o.rating, conditions: o.conditions, report: o.report })
  const [busy, setBusy] = useState('')
  const file = useRef<HTMLInputElement>(null)
  const dirty = f.rating !== o.rating || f.conditions !== o.conditions || f.report !== o.report
  const save = async () => {
    setBusy('save')
    try { onPage(await api.put<Page>(`${V1}/outings/${o.id}`, f)); announceChange(); toast('Saved', 'ok') } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const call = async (what: string, fn: () => Promise<Page>) => { setBusy(what); try { onPage(await fn()) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') } }
  const draft = async () => {
    setBusy('ai')
    try { const r = await api.post<{ text: string }>(`${V1}/ai`, { want: 'report', outing_id: o.id, hint: f.conditions }); setF(x => ({ ...x, report: r.text })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const c = o.actual
  const pace = c?.speed_pct && c.moving_s > 1200 ? c.speed_pct : 0
  const setPace = async () => { try { setSettings(await api.put(`${V1}/settings`, { pace_pct: pace })); toast(`Times now at ${pace}% of the usual speed`, 'ok') } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Card title={<span className="sec-title"><Icon name="check-circle" size={15} />How did it go?</span>}>
      <div className="stack">
        {c && c.points > 1 ? <>
          <div className="stats-row tight">
            <div className="stat"><span className="stat-l">Walked</span><span className="stat-v">{fmtDist(c.distance_m, u)}</span>{c.planned_m > 0 && <span className="stat-s tiny">planned {fmtDist(c.planned_m, u)}</span>}</div>
            <div className="stat"><span className="stat-l">Up</span><span className="stat-v">{c.has_ele ? fmtHeight(c.gain_m, u) : '—'}</span></div>
            <div className="stat"><span className="stat-l">Moving</span><span className="stat-v">{fmtSeconds(c.moving_s)}</span><span className="stat-s tiny">{fmtSeconds(c.total_s)} in all</span></div>
            {c.planned_m > 0 && <div className="stat"><span className="stat-l">Of the plan</span><span className="stat-v">{c.covered_pct}%</span><span className="stat-s tiny">{c.max_off_m > 150 ? `up to ${fmtDist(c.max_off_m, u)} off it` : 'on the way'}</span></div>}
          </div>
          {pace > 0 && Math.abs(pace - (settings?.pace_pct || 100)) >= 8 && <p className="small m0 tip-line"><Icon name="timer" size={13} className="inline-ico" /> On the move you were {pace > 100 ? `${pace - 100}% faster` : `${100 - pace}% slower`} than the usual times. <button type="button" className="linkish" onClick={setPace}>Plan with {pace}%</button></p>}
          <div className="row wrap">
            <span className="muted small">Track {o.actual_source === 'movement' ? 'from Pocket Movement' : 'from a file'}{page.links.movement ? <> · <a href={page.links.movement} target="_blank" rel="noreferrer">open the day</a></> : null}</span>
            <span className="spacer" />
            <button type="button" className="btn ghost sm" onClick={() => call('route', async () => { const rt = await api.post<{ id: number }>(`${V1}/outings/${o.id}/track/route`); toast('Saved as a route', 'ok', { label: 'Open', run: () => window.location.assign(`/routes/${rt.id}`) }); return api.get<Page>(`${V1}/outings/${o.id}`) })} disabled={!!busy}><Icon name="route" size={13} />Save as a route</button>
            <button type="button" className="btn ghost sm" onClick={() => call('forget', () => api.del<Page>(`${V1}/outings/${o.id}/track`))} disabled={!!busy}><Icon name="trash" size={13} />Forget the track</button>
          </div>
        </> : <div className="track-get">
          <p className="muted small m0">Compare what you walked with the plan:</p>
          <div className="row wrap">
            <button type="button" className="btn sm" disabled={!movementRead.ok || !!busy} title={movementRead.ok ? 'Take the track Pocket Movement recorded' : offTitle(movementRead.app, 'movement')}
              onClick={() => call('mv', () => api.post<Page>(`${V1}/outings/${o.id}/track/movement`))}>{busy === 'mv' ? <Spinner /> : <Icon name="navigation" size={13} />}From Pocket Movement</button>
            <button type="button" className="btn sm" onClick={() => file.current?.click()} disabled={!!busy}>{busy === 'gpx' ? <Spinner /> : <Icon name="upload" size={13} />}From a GPX file</button>
            <input ref={file} type="file" hidden accept=".gpx,.kml,.tcx,.geojson,.json" onChange={e => { const fl = e.target.files?.[0]; e.target.value = ''; if (fl) call('gpx', () => upload<Page>(`${V1}/outings/${o.id}/track?name=${encodeURIComponent(fl.name)}`, 'file', fl)) }} />
          </div>
          {!movementRead.ok && !movement.loading && movementRead.app?.state !== 'missing' && <p className="muted tiny m0">{movementRead.app?.fix}</p>}
        </div>}
        <div className="row"><span className="muted small">How was it</span><Stars value={f.rating} onChange={n => setF(x => ({ ...x, rating: n }))} /></div>
        <label className="field"><span className="field-label">Trail conditions, for next time</span><textarea rows={2} value={f.conditions} onChange={e => setF(x => ({ ...x, conditions: e.target.value }))} placeholder="Muddy after the hut; the spring was dry." /></label>
        <label className="field"><span className="field-label row tight">The story{settings?.ai_help && <button type="button" className="btn ghost sm" onClick={draft} disabled={!!busy}>{busy === 'ai' ? <Spinner /> : <Icon name="sparkles" size={13} />}Draft it for me</button>}</span>
          <textarea rows={4} value={f.report} onChange={e => setF(x => ({ ...x, report: e.target.value }))} placeholder="What you saw, who you met, what you would do differently." /></label>
        <div className="row end"><button type="button" className="btn primary" onClick={save} disabled={!dirty || busy === 'save'}>{busy === 'save' ? <Spinner /> : <Icon name="check" size={15} />}Save</button></div>
      </div>
    </Card>
  )
}

// ---- what to pack ----

function Packing({ page, onPage }: { page: Page; onPage: (p: Page) => void }) {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const appState = useAppState()
  const tasks = appState('tasks', 'tasks:write')
  const o = page.outing
  const items = o.items || []
  const { data: gear } = useAsync(() => api.get<Gear[]>(`${V1}/gear`), [])
  const { data: packs } = useAsync(() => api.get<Pack[]>(`${V1}/packs`), [])
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState('')
  const [ideas, setIdeas] = useState<{ name: string; category: GearKind; why: string; gear_id?: number; weight_g?: number }[] | null>(null)
  const setItems = (list: OutingItem[]) => onPage({ ...page, outing: { ...o, items: list, item_count: list.length, packed_count: list.filter(i => i.packed).length } })
  const add = async (body: Record<string, unknown>) => {
    try { setItems(await api.post<OutingItem[]>(`${V1}/outings/${o.id}/items`, body)); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const patch = async (it: OutingItem, b: Partial<OutingItem>) => {
    setItems(items.map(x => x.id === it.id ? { ...x, ...b } : x))
    try { await api.put(`${V1}/items/${it.id}`, b); announceChange() } catch (e) { toast(errMsg(e), 'err'); setItems(items) }
  }
  const drop = async (it: OutingItem) => { setItems(items.filter(x => x.id !== it.id)); try { await api.del(`${V1}/items/${it.id}`) } catch (e) { toast(errMsg(e), 'err') } }
  const all = async (packed: boolean) => { try { setItems(await api.post<OutingItem[]>(`${V1}/outings/${o.id}/items/all`, { packed })); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const suggest = async () => { setBusy('ai'); try { setIdeas((await api.post<{ items: NonNullable<typeof ideas> }>(`${V1}/ai`, { want: 'pack', outing_id: o.id })).items) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') } }
  const toTasks = async () => {
    setBusy('tasks')
    try { const p = await api.post<Page>(`${V1}/outings/${o.id}/make/tasks`); onPage(p); toast(p.message || 'Done', 'ok', p.open_url ? { label: 'Open', run: () => window.open(p.open_url, '_blank') } : undefined) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const groups = useMemo(() => GEAR_KINDS.map(k => [k, items.filter(i => i.category === k)] as const).filter(([, l]) => l.length), [items])
  const w = items.reduce((acc, i) => { const t = i.weight_g * i.qty; if (i.worn) acc.worn += t; else if (i.consumable) acc.cons += t; else acc.base += t; return acc }, { base: 0, worn: 0, cons: 0 })
  const toBuy = items.filter(i => i.to_buy && !i.task_ref).length
  const done = items.length > 0 && items.every(i => i.packed)
  const have = new Set(items.map(i => i.gear_id).filter(Boolean))
  return (
    <Card title={<span className="sec-title" id="pack"><Icon name="backpack" size={15} />What to pack{items.length > 0 && <span className="count">{items.filter(i => i.packed).length}/{items.length}</span>}</span>}
      actions={items.length > 0 && <label className="check-row small"><Switch size="sm" checked={done} onChange={all} />All packed</label>} flush>
      {items.length > 0 && <div className="pack-weights"><span>In the pack <b>{fmtWeight(w.base + w.cons, u)}</b></span><span className="muted">base {fmtWeight(w.base, u)}</span><span className="muted">food and water {fmtWeight(w.cons, u)}</span><span className="muted">worn {fmtWeight(w.worn, u)}</span></div>}
      {groups.map(([k, list]) => (
        <div key={k} className="pack-group">
          <h5 className="pack-h"><Icon name={GEAR[k].icon} size={13} />{GEAR[k].label}</h5>
          <ul className="list">{list.map(it => (
            <li key={it.id} className={`list-row pack-row ${it.packed ? 'packed' : ''}`}>
              <input type="checkbox" checked={it.packed} onChange={e => patch(it, { packed: e.target.checked })} aria-label={`Packed: ${it.name}`} />
              <span className="row-main grow"><span className="row-title">{it.name}{it.qty > 1 && <span className="muted"> ×{it.qty}</span>}</span>{(it.worn || it.to_buy) && <span className="muted tiny">{it.worn ? 'worn' : ''}{it.to_buy ? (it.task_ref ? ' · on your task list' : ' · to buy') : ''}</span>}</span>
              <span className="muted small tnum">{it.weight_g ? fmtWeight(it.weight_g * it.qty, u) : ''}</span>
              <button type="button" className={`btn ghost icon-only sm ${it.to_buy ? 'on' : ''}`} aria-pressed={it.to_buy} title={it.to_buy ? 'Still to buy' : 'Mark as still to buy'} onClick={() => patch(it, { to_buy: !it.to_buy })}><Icon name="cart" size={14} /></button>
              <button type="button" className="btn ghost icon-only sm" aria-label={`Remove ${it.name}`} onClick={() => drop(it)}><Icon name="x" size={14} /></button>
            </li>))}</ul>
        </div>
      ))}
      <div className="pad stack pack-add">
        {!items.length && <p className="muted small m0">Nothing on the list yet. Start from one of your packing lists, pick your gear, or type what to take.</p>}
        <div className="row wrap tight">
          {packs && packs.length > 0 && <Select ariaLabel="Add a packing list" value="" placeholder="Add a packing list…" onChange={v => v && add({ pack_id: Number(v) })} options={packs.map(p => ({ value: String(p.id), label: p.name, hint: `${p.count} things` }))} />}
          {gear && gear.length > 0 && <Select ariaLabel="Add gear" value="" placeholder="Add your gear…" onChange={v => v && add({ gear_id: Number(v) })} options={gear.filter(g => !have.has(g.id)).map(g => ({ value: String(g.id), label: g.name, hint: fmtWeight(g.weight_g, u) }))} />}
        </div>
        <form className="row tight" onSubmit={e => { e.preventDefault(); if (typed.trim()) { add({ name: typed.trim() }); setTyped('') } }}>
          <input value={typed} onChange={e => setTyped(e.target.value)} placeholder="Something else to take" aria-label="Something else to take" />
          <button type="submit" className="btn" disabled={!typed.trim()}>Add</button>
        </form>
        <div className="row wrap">
          {settings?.ai_help && <button type="button" className="btn ghost sm" onClick={suggest} disabled={!!busy}>{busy === 'ai' ? <Spinner /> : <Icon name="sparkles" size={13} />}What else should I take?</button>}
          {toBuy > 0 && <button type="button" className="btn ghost sm" onClick={toTasks} disabled={!tasks.ok || !!busy} title={tasks.ok ? 'Put what you still need to buy on your task list' : offTitle(tasks.app, 'tasks')}>{busy === 'tasks' ? <Spinner /> : <Icon name="check-square" size={13} />}{toBuy} to buy → Pocket Tasks</button>}
        </div>
        {ideas && <div className="ideas">{ideas.length === 0 ? <p className="muted small m0">Nothing to add — the list looks complete.</p> : ideas.map((x, i) => (
          <div key={i} className="idea-row"><span className="grow"><b>{x.name}</b> <span className="muted small">{x.why}</span></span>
            <button type="button" className="btn sm" onClick={() => { add(x.gear_id ? { gear_id: x.gear_id } : { name: x.name, category: x.category }); setIdeas(l => l!.filter((_, j) => j !== i)) }}><Icon name="plus" size={13} />Add</button></div>
        ))}</div>}
      </div>
    </Card>
  )
}

// ---- the other apps ----

function OtherApps({ page, onPage }: { page: Page; onPage: (p: Page) => void }) {
  const { toast } = useToast()
  const appState = useAppState()
  const [busy, setBusy] = useState('')
  const o = page.outing
  const make = async (app: string) => {
    setBusy(app)
    try { const p = await api.post<Page>(`${V1}/outings/${o.id}/make/${app}`); onPage(p); toast(p.message || 'Done', 'ok', p.open_url ? { label: 'Open', run: () => window.open(p.open_url, '_blank') } : undefined) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const row = (slug: 'calendar' | 'health' | 'notes', scope: string, icon: string, done: boolean, doneText: string, link: string | undefined, label: string, show: boolean) => {
    if (!show) return null
    const st = appState(slug, scope)
    return (
      <li className="else-row">
        <Icon name={icon} size={15} className="muted" />
        {done ? <span className="grow small">{link ? <a href={link} target="_blank" rel="noreferrer">{doneText}</a> : doneText}</span>
          : <><span className="grow small muted">{label}</span><button type="button" className="btn sm" disabled={!st.ok || !!busy} title={st.ok ? undefined : offTitle(st.app, slug)} onClick={() => make(slug)}>{busy === slug ? <Spinner /> : 'Do it'}</button></>}
      </li>
    )
  }
  const live = o.status === 'planned' || o.status === 'out'
  return (
    <Card className="side-card">
      <h4 className="side-h">In your other apps</h4>
      <ul className="else-list">
        {row('calendar', 'calendar:write', 'calendar', !!o.cal_ref, 'In Pocket Calendar', page.links.calendar, 'Put it in Pocket Calendar', live || !!o.cal_ref)}
        {row('health', 'health:write', 'heart-pulse', !!o.health_ref, 'Logged in Pocket Health', page.links.health, 'Log it in Pocket Health', o.status === 'done')}
        {row('notes', 'notes:write', 'note', !!o.note_ref, 'Saved in Pocket Notes', page.links.notes, 'Save how it went in Pocket Notes', o.status === 'done')}
      </ul>
    </Card>
  )
}
