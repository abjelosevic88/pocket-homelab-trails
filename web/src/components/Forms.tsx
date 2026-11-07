// The forms: an outing (who comes, when to be back and who to tell), a
// route's details, a spot along it, gear, a packing list, and the ways a
// route comes in (a file, a day Pocket Movement recorded). Each works on
// its own; what another Pocket app adds is offered when it is connected
// and greyed with what to do when it is not.
import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, uploadMany, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState } from '../lib/data'
import { fmtDate, todayISO } from '../lib/format'
import { ACTIVITIES, ACTIVITY, GEAR, GEAR_KINDS, LEVEL, WAYPOINT, WAYPOINT_KINDS, fmtDist, fmtHeight, fmtMinutes, fmtWeight } from '../lib/trails'
import type { Activity, ContactHit, Gear, GearKind, MovementTrack, Outing, OutingPage, Pack, Person, Route, Trip, Waypoint, WaypointKind } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Segmented, Spinner, Switch, errMsg, useAsync, useDebounce, useToast } from './ui'

type Init = Record<string, unknown>

/** Find someone in Pocket Contacts. */
export function ContactSearch({ onPick, placeholder = 'Find in Pocket Contacts…' }: { onPick: (c: ContactHit) => void; placeholder?: string }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<ContactHit[] | null>(null)
  const [off, setOff] = useState('')
  useEffect(() => {
    if (dq.trim().length < 2) { setList(null); return }
    let alive = true
    api.get<{ contacts: ContactHit[]; off?: string }>(`${V1}/apps/contacts?q=${encodeURIComponent(dq.trim())}`)
      .then(r => { if (alive) { setList(r.contacts); setOff(r.off || '') } }).catch(e => { if (alive) setOff(errMsg(e)) })
    return () => { alive = false }
  }, [dq])
  return (
    <div className="picker">
      <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder={placeholder} aria-label={placeholder} /></div>
      {off && <p className="muted small">{off}</p>}
      {list && (list.length === 0 ? <p className="muted small">Nobody called “{dq}” in your contacts.</p> : (
        <div className="pick-list">
          {list.map(c => (
            <button key={c.id} type="button" className="pick-row" onClick={() => { onPick(c); setQ(''); setList(null) }}>
              <span className="pick-ava" style={{ background: c.color || 'var(--accent)' }}>{c.initials || c.name.slice(0, 1)}</span>
              <span className="grow ellipsis"><b>{c.name}</b>{c.email && <span className="muted"> · {c.email}</span>}</span>
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}

function routeOption(r: Route, units: 'metric' | 'imperial') {
  return { value: String(r.id), label: r.name, hint: `${fmtDist(r.distance_m, units)} · ${fmtHeight(r.gain_m, units)} up` }
}

function addMinutes(date: string, hm: string, minutes: number): string {
  const [y, m, d] = date.split('-').map(Number)
  const [h, mi] = (hm || '09:00').split(':').map(Number)
  const t = new Date(y, m - 1, d, h, mi + minutes)
  const r = t.getMinutes() % 15
  if (r) t.setMinutes(t.getMinutes() + 15 - r)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())}T${pad(t.getHours())}:${pad(t.getMinutes())}`
}

// ---- an outing ----

export function OutingForm({ outing, initial = {}, onClose, onSaved }: { outing?: Outing; initial?: Init; onClose: () => void; onSaved?: (p: OutingPage) => void }) {
  const { settings } = useApp()
  const units = settings?.units || 'metric'
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  const travel = appState('travel', 'trips:read')
  const { data: routes } = useAsync(() => api.get<Route[]>(`${V1}/routes`), [])
  const { data: packs } = useAsync(() => outing ? Promise.resolve([] as Pack[]) : api.get<Pack[]>(`${V1}/packs`), [])
  const [f, setF] = useState(() => ({
    route_id: String(outing?.route_id ?? initial.route_id ?? ''), title: outing?.title || '', activity: (outing?.activity || initial.activity || settings?.default_activity || 'hike') as Activity,
    date: outing?.date || (initial.date as string) || todayISO(), start_time: outing?.start_time ?? '09:00', notes: outing?.notes || '',
    back_date: outing?.back_by ? outing.back_by.slice(0, 10) : '', back_time: outing?.back_by ? outing.back_by.slice(11, 16) : '',
    safety: outing ? outing.safety : !!settings?.safety_default,
    contact_name: outing ? outing.contact_name : settings?.contact_name || '', contact_email: outing ? outing.contact_email : settings?.contact_email || '',
    contact_ref: outing ? outing.contact_ref : settings?.contact_ref || '', alert_after: outing?.alert_after ?? settings?.alert_after ?? 60,
    trip_ref: outing?.trip_ref || '', trip_name: outing?.trip_name || '', pack_id: '',
  }))
  const [people, setPeople] = useState<Person[]>(outing?.people || [])
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof typeof f>(k: K, v: (typeof f)[K]) => setF(x => ({ ...x, [k]: v }))
  const route = routes?.find(r => String(r.id) === f.route_id)
  const { data: trips } = useAsync(() => travel.ok ? api.get<Trip[]>(`${V1}/apps/options/trips?date=${f.date}`).catch(() => [] as Trip[]) : Promise.resolve([] as Trip[]), [travel.ok, f.date])
  // the route sets the activity; the route's time suggests when to be back
  useEffect(() => { if (route && !outing) set('activity', route.activity) }, [route?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const suggested = route && f.start_time ? addMinutes(f.date, f.start_time, Math.round(route.minutes * 1.2 + 60)) : ''
  useEffect(() => {
    if (!outing && suggested && f.safety) { setF(x => ({ ...x, back_date: suggested.slice(0, 10), back_time: suggested.slice(11, 16) })) }
  }, [suggested, f.safety]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!outing && packs?.length && !f.pack_id) {
      const p = packs.find(p => p.activity === f.activity) || packs.find(p => !p.activity)
      if (p) set('pack_id', String(p.id))
    }
  }, [packs, f.activity]) // eslint-disable-line react-hooks/exhaustive-deps
  const addPerson = (p: Person) => { if (p.name || p.email) setPeople(ps => ps.some(x => (x.ref && x.ref === p.ref) || (x.name && x.name === p.name)) ? ps : [...ps, p]) }
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (typed.trim()) addPerson({ name: typed.trim() })
    setBusy(true)
    const back = f.back_date && f.back_time ? `${f.back_date}T${f.back_time}` : ''
    const body = {
      route_id: f.route_id ? Number(f.route_id) : null, title: f.title, activity: f.activity, date: f.date, start_time: f.start_time, notes: f.notes,
      people: typed.trim() ? [...people, { name: typed.trim() }] : people, back_by: back, safety: f.safety && !!back,
      contact_name: f.contact_name, contact_email: f.contact_email, contact_ref: f.contact_ref, alert_after: Number(f.alert_after) || 0,
      trip_ref: f.trip_ref, trip_name: f.trip_name, ...(outing ? {} : { pack_id: Number(f.pack_id) || 0 }),
    }
    try {
      const p = outing ? await api.put<OutingPage>(`${V1}/outings/${outing.id}`, body) : await api.post<OutingPage>(`${V1}/outings`, body)
      announceChange()
      toast(outing ? 'Saved' : 'Planned', 'ok')
      onSaved?.(p)
      onClose()
      if (!outing) navigate(`/outings/${p.outing.id}`)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const safetyOk = !f.safety || (!!f.back_date && !!f.back_time)
  return (
    <Modal title={outing ? 'Change the outing' : 'Plan an outing'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.date || !safetyOk}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{outing ? 'Save' : 'Plan it'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="Route" group help={routes && !routes.length ? 'You have no routes yet: bring one in from a file or draw one (the + menu). You can also plan without a route.' : undefined}>
          <Select ariaLabel="Route" value={f.route_id} onChange={v => set('route_id', v)} placeholder="No route (just the day)" clearLabel="No route"
            options={(routes || []).map(r => routeOption(r, units))} />
        </Field>
        {route && <p className="muted small route-facts"><Icon name={ACTIVITY[route.activity].icon} size={13} className="inline-ico" /> {fmtDist(route.distance_m, units)} · {fmtHeight(route.gain_m, units)} up · about {fmtMinutes(route.minutes)}{route.level && ` · ${LEVEL[route.level].label.toLowerCase()}`}</p>}
        <div className="three-col tight">
          <Field label="Day"><input type="date" value={f.date} onChange={e => set('date', e.target.value)} /></Field>
          <Field label="Setting off"><input type="time" value={f.start_time} onChange={e => set('start_time', e.target.value)} /></Field>
          <Field label="What" group><Select ariaLabel="Activity" value={f.activity} onChange={v => set('activity', v as Activity)} options={ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))} /></Field>
        </div>
        <Field label="Name" help="Leave empty to use the route’s name."><input value={f.title} onChange={e => set('title', e.target.value)} placeholder={route ? `${ACTIVITY[f.activity].label}: ${route.name}` : 'Saturday hike'} /></Field>

        <section className="form-sec">
          <h4 className="form-sec-h"><Icon name="users" size={13} />Who comes along</h4>
          {people.length > 0 && <div className="chips">{people.map((p, i) => <span key={i} className="chip">{p.name || p.email}<button type="button" className="chip-x" aria-label={`Remove ${p.name}`} onClick={() => setPeople(ps => ps.filter((_, j) => j !== i))}><Icon name="x" size={12} /></button></span>)}</div>}
          {contacts.ok && <ContactSearch onPick={c => addPerson({ name: c.name, email: c.email, ref: c.id })} />}
          <div className="row tight"><input value={typed} onChange={e => setTyped(e.target.value)} placeholder={contacts.ok ? 'Or type a name' : 'Type a name'} aria-label="Name"
            onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); if (typed.trim()) { addPerson({ name: typed.trim() }); setTyped('') } } }} />
            <button type="button" className="btn" disabled={!typed.trim()} onClick={() => { addPerson({ name: typed.trim() }); setTyped('') }}>Add</button></div>
          {!contacts.ok && contacts.app && contacts.app.state !== 'missing' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> Connect Pocket Contacts (Settings → Connected apps) to pick people from your contacts.</p>}
        </section>

        <section className="form-sec">
          <h4 className="form-sec-h"><Icon name="shield" size={13} />Be back by, and who to tell</h4>
          <label className="check-row"><Switch checked={f.safety} onChange={v => set('safety', v)} /><span>Tell someone if I am not back in time<span className="muted small block-line">You get a message at the time; if you have not tapped “I'm back” a while later, they get an email with your plan.</span></span></label>
          <div className="two-col tight">
            <Field label="Back by"><div className="row tight"><input type="date" value={f.back_date} onChange={e => set('back_date', e.target.value)} aria-label="Back by, day" /><input type="time" value={f.back_time} onChange={e => set('back_time', e.target.value)} aria-label="Back by, time" /></div></Field>
            {f.safety && <Field label="Tell them after" group><Select ariaLabel="Wait" value={String(f.alert_after)} onChange={v => set('alert_after', Number(v))} options={[0, 15, 30, 60, 90, 120, 180, 240].map(m => ({ value: String(m), label: m ? `${fmtMinutes(m)} late` : 'At once' }))} /></Field>}
          </div>
          {suggested && !f.back_date && <button type="button" className="linkish self-start small" onClick={() => setF(x => ({ ...x, back_date: suggested.slice(0, 10), back_time: suggested.slice(11, 16) }))}>Suggest: {suggested.slice(11, 16)} (the route’s time, a fifth for stops and an hour)</button>}
          {f.safety && <>
            <div className="two-col tight">
              <Field label="Who to tell"><input value={f.contact_name} onChange={e => set('contact_name', e.target.value)} placeholder="Name" /></Field>
              <Field label="Their email"><input type="email" value={f.contact_email} onChange={e => set('contact_email', e.target.value)} placeholder="name@example.com" /></Field>
            </div>
            {contacts.ok && <ContactSearch placeholder="Pick them from Pocket Contacts…" onPick={c => setF(x => ({ ...x, contact_name: c.name, contact_email: c.email || x.contact_email, contact_ref: c.id }))} />}
          </>}
        </section>

        {(travel.ok || f.trip_ref) && trips && trips.length > 0 && <Field label="Part of a trip" group help="From Pocket Travel.">
          <Select ariaLabel="Trip" value={f.trip_ref} onChange={v => { const t = trips.find(x => x.id === v); setF(x => ({ ...x, trip_ref: v, trip_name: t?.name || '' })) }} placeholder="Not part of a trip" clearLabel="Not part of a trip"
            options={trips.map(t => ({ value: t.id, label: t.name, hint: t.during ? 'on that day' : t.start_date ? fmtDate(t.start_date) : '' }))} />
        </Field>}
        {!outing && packs && packs.length > 0 && <Field label="Start the packing list from" group>
          <Select ariaLabel="Packing list" value={f.pack_id} onChange={v => set('pack_id', v)} placeholder="Nothing yet" clearLabel="Nothing yet"
            options={packs.map(p => ({ value: String(p.id), label: p.name, hint: `${p.count} things · ${fmtWeight(p.weight.total, units)}` }))} />
        </Field>}
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => set('notes', e.target.value)} placeholder="Meet at the petrol station at 7:45." /></Field>
      </form>
    </Modal>
  )
}

// ---- a route's details ----

export function RouteForm({ route, onClose, onSaved }: { route: Route; onClose: () => void; onSaved: (r: Route) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState(() => ({
    name: route.name, activity: route.activity, difficulty: route.difficulty, region: route.region, start_name: route.start_name, description: route.description,
    tags: route.tags.join(', '), link_url: route.link_url, own: route.minutes_set > 0, h: Math.floor((route.minutes_set || route.minutes) / 60), m: (route.minutes_set || route.minutes) % 60,
  }))
  const [busy, setBusy] = useState(false)
  const [ai, setAi] = useState(false)
  const set = <K extends keyof typeof f>(k: K, v: (typeof f)[K]) => setF(x => ({ ...x, [k]: v }))
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const p = await api.put<{ route: Route }>(`${V1}/routes/${route.id}`, {
        name: f.name, activity: f.activity, difficulty: f.difficulty, region: f.region, start_name: f.start_name, description: f.description,
        tags: f.tags.split(',').map(t => t.trim()).filter(Boolean), link_url: f.link_url, minutes_set: f.own ? Number(f.h) * 60 + Number(f.m) : 0,
      })
      announceChange()
      onSaved(p.route)
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const describe = async () => {
    setAi(true)
    try { const r = await api.post<{ text: string }>(`${V1}/ai`, { want: 'describe', route_id: route.id }); set('description', r.text) } catch (err) { toast(errMsg(err), 'err') } finally { setAi(false) }
  }
  return (
    <Modal title="Change the route" onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.name.trim()}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="Name"><input value={f.name} onChange={e => set('name', e.target.value)} autoFocus /></Field>
        <div className="two-col tight">
          <Field label="What" group><Select ariaLabel="Activity" value={f.activity} onChange={v => set('activity', v as Activity)} options={ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))} /></Field>
          <Field label="How hard" group help={`Worked out from the length and the climb: ${route.level ? LEVEL[route.level].label.toLowerCase() : '—'}.`}>
            <Select ariaLabel="Difficulty" value={f.difficulty} onChange={v => set('difficulty', v as Route['difficulty'])} options={[{ value: '', label: 'Worked out' }, ...(['easy', 'moderate', 'hard', 'expert'] as const).map(l => ({ value: l, label: LEVEL[l].label }))]} />
          </Field>
          <Field label="Region"><input value={f.region} onChange={e => set('region', e.target.value)} placeholder="Bjelašnica" /></Field>
          <Field label="Where it starts"><input value={f.start_name} onChange={e => set('start_name', e.target.value)} placeholder="Babin Do car park" /></Field>
        </div>
        <section className="form-sec">
          <label className="check-row"><Switch checked={f.own} onChange={v => set('own', v)} /><span>Use my own time<span className="muted small block-line">Worked out: about {fmtMinutes(route.minutes_set ? 0 : route.minutes) !== '—' ? fmtMinutes(route.minutes) : 'the usual'} at your speed ({settings?.pace_pct || 100}% — Settings → General).</span></span></label>
          {f.own && <div className="row tight"><input type="number" min={0} className="num-sm" value={f.h} onChange={e => set('h', Number(e.target.value))} aria-label="Hours" /><span className="muted">h</span><input type="number" min={0} max={59} className="num-sm" value={f.m} onChange={e => set('m', Number(e.target.value))} aria-label="Minutes" /><span className="muted">min</span></div>}
        </section>
        <Field label="Tags" help="Separate with commas."><input value={f.tags} onChange={e => set('tags', e.target.value)} placeholder="summit, with kids" /></Field>
        <Field label="Link" help="Where you found it (Komoot, AllTrails, a blog)."><input value={f.link_url} onChange={e => set('link_url', e.target.value)} placeholder="https://" /></Field>
        <Field label={<span className="row tight">Description{settings?.ai_help && <button type="button" className="btn ghost sm" onClick={describe} disabled={ai}>{ai ? <Spinner /> : <Icon name="sparkles" size={13} />}Write it for me</button>}</span>}>
          <textarea rows={4} value={f.description} onChange={e => set('description', e.target.value)} />
        </Field>
      </form>
    </Modal>
  )
}

// ---- a spot along a route ----

export function WaypointForm({ routeId, waypoint, at, onClose, onSaved }: { routeId: number; waypoint?: Waypoint; at?: [number, number]; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const appState = useAppState()
  const places = appState('places', 'places:write')
  const [f, setF] = useState({ kind: (waypoint?.kind || 'water') as WaypointKind, name: waypoint?.name || '', note: waypoint?.note || '' })
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try {
      if (waypoint) await api.put(`${V1}/waypoints/${waypoint.id}`, f)
      else await api.post(`${V1}/routes/${routeId}/waypoints`, { ...f, lat: at![0], lng: at![1] })
      onSaved(); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const del = async () => { if (!waypoint) return; setBusy(true); try { await api.del(`${V1}/waypoints/${waypoint.id}`); onSaved(); onClose() } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) } }
  const toPlaces = async () => {
    if (!waypoint) return
    setBusy(true)
    try { const r = await api.post<{ url: string }>(`${V1}/waypoints/${waypoint.id}/place`); toast('Saved in Pocket Places', 'ok', r.url ? { label: 'Open', run: () => window.open(r.url, '_blank') } : undefined); onSaved() } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={waypoint ? (waypoint.name || WAYPOINT[waypoint.kind].label) : 'A spot on the way'} onClose={onClose} footer={<>
      {waypoint && <button type="button" className="btn danger-ghost" onClick={del} disabled={busy}><Icon name="trash" size={14} />Remove</button>}
      <span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <div className="stack">
        <div className="kind-grid" role="group" aria-label="What is there">
          {WAYPOINT_KINDS.map(k => <button key={k} type="button" className={`kind-btn ${f.kind === k ? 'on' : ''}`} aria-pressed={f.kind === k} onClick={() => setF(x => ({ ...x, kind: k }))}><Icon name={WAYPOINT[k].icon} size={16} />{WAYPOINT[k].label}</button>)}
        </div>
        <Field label="Name"><input value={f.name} onChange={e => setF(x => ({ ...x, name: e.target.value }))} placeholder={WAYPOINT[f.kind].label} autoFocus /></Field>
        <Field label="Note"><textarea rows={2} value={f.note} onChange={e => setF(x => ({ ...x, note: e.target.value }))} placeholder={f.kind === 'water' ? 'Runs until August' : ''} /></Field>
        {waypoint && <div className="row">
          {waypoint.place_ref ? <span className="muted small"><Icon name="check" size={12} className="inline-ico" /> In Pocket Places</span>
            : <button type="button" className="btn sm" disabled={!places.ok || busy} title={places.ok ? 'Save it as a place you want to go to' : 'Connect Pocket Places (Settings → Connected apps)'} onClick={toPlaces}><Icon name="pin" size={13} />Save in Pocket Places</button>}
        </div>}
      </div>
    </Modal>
  )
}

// ---- gear ----

export function GearForm({ gear, onClose, onSaved }: { gear?: Gear; onClose: () => void; onSaved?: () => void }) {
  const { settings } = useApp()
  const imperial = settings?.units === 'imperial'
  const { toast } = useToast()
  const [f, setF] = useState({ name: gear?.name || '', category: (gear?.category || 'other') as GearKind, w: gear ? (imperial ? (gear.weight_g / 28.3495).toFixed(1) : String(gear.weight_g)) : '', worn: !!gear?.worn, consumable: !!gear?.consumable, note: gear?.note || '' })
  const [busy, setBusy] = useState(false)
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    const g = Math.round((Number(f.w.replace(',', '.')) || 0) * (imperial ? 28.3495 : 1))
    try {
      const body = { name: f.name, category: f.category, weight_g: g, worn: f.worn, consumable: f.consumable, note: f.note }
      if (gear) await api.put(`${V1}/gear/${gear.id}`, body)
      else await api.post(`${V1}/gear`, gear ? body : { ...body, category: f.category === 'other' ? undefined : f.category })
      announceChange(); window.dispatchEvent(new CustomEvent('ptr:gear'))
      onSaved?.(); onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={gear ? 'Change gear' : 'Add gear'} onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !f.name.trim()}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{gear ? 'Save' : 'Add'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="What"><input value={f.name} onChange={e => setF(x => ({ ...x, name: e.target.value }))} placeholder="Head torch" autoFocus /></Field>
        <div className="two-col tight">
          <Field label="Kind" group><Select ariaLabel="Kind" value={f.category} onChange={v => setF(x => ({ ...x, category: v as GearKind }))} options={GEAR_KINDS.map(k => ({ value: k, label: GEAR[k].label }))} /></Field>
          <Field label="Weight" group><div className="input-group"><input inputMode="decimal" value={f.w} onChange={e => setF(x => ({ ...x, w: e.target.value }))} placeholder="0" aria-label="Weight" /><span className="input-suffix">{imperial ? 'oz' : 'g'}</span></div></Field>
        </div>
        <label className="check-row"><Switch checked={f.worn} onChange={v => setF(x => ({ ...x, worn: v }))} /><span>I wear it<span className="muted small block-line">Boots, the clothes you walk in: not in the pack’s weight.</span></span></label>
        <label className="check-row"><Switch checked={f.consumable} onChange={v => setF(x => ({ ...x, consumable: v }))} /><span>It gets used up<span className="muted small block-line">Food, water, fuel.</span></span></label>
        <Field label="Note"><input value={f.note} onChange={e => setF(x => ({ ...x, note: e.target.value }))} /></Field>
      </form>
    </Modal>
  )
}

// ---- a packing list ----

export function PackForm({ pack, onClose }: { pack?: Pack; onClose: () => void }) {
  const { settings } = useApp()
  const units = settings?.units || 'metric'
  const { toast } = useToast()
  const { data: gear } = useAsync(() => api.get<Gear[]>(`${V1}/gear`), [])
  const [name, setName] = useState(pack?.name || '')
  const [activity, setActivity] = useState<string>(pack?.activity || '')
  const [qty, setQty] = useState<Record<number, number>>(() => Object.fromEntries((pack?.items || []).map(i => [i.gear_id, i.qty])))
  const [busy, setBusy] = useState(false)
  const byCat = useMemo(() => {
    const m = new Map<GearKind, Gear[]>()
    for (const g of gear || []) m.set(g.category, [...(m.get(g.category) || []), g])
    return [...m.entries()]
  }, [gear])
  const total = (gear || []).reduce((s, g) => s + (qty[g.id] && !g.worn ? g.weight_g * qty[g.id] : 0), 0)
  const save = async () => {
    setBusy(true)
    const items = Object.entries(qty).filter(([, q]) => q > 0).map(([id, q]) => ({ gear_id: Number(id), qty: q }))
    try {
      if (pack) await api.put(`${V1}/packs/${pack.id}`, { name, activity, items })
      else await api.post(`${V1}/packs`, { name, activity, items })
      window.dispatchEvent(new CustomEvent('ptr:gear'))
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={pack ? 'Change the packing list' : 'A new packing list'} onClose={onClose} size="lg" footer={<>
      <span className="muted small">In the pack: <b>{fmtWeight(total, units)}</b></span><span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy || !name.trim()}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
    </>}>
      <div className="stack">
        <div className="two-col tight">
          <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} placeholder="Day hike in summer" autoFocus /></Field>
          <Field label="For" group><Select ariaLabel="Activity" value={activity} onChange={setActivity} options={[{ value: '', label: 'Anything' }, ...ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))]} /></Field>
        </div>
        {gear && !gear.length && <p className="muted">Add your gear first (Gear → Add gear), then pick it here.</p>}
        {byCat.map(([cat, list]) => (
          <section key={cat} className="form-sec">
            <h4 className="form-sec-h"><Icon name={GEAR[cat].icon} size={13} />{GEAR[cat].label}</h4>
            {list.map(g => (
              <label key={g.id} className="check-row pack-pick">
                <input type="checkbox" checked={!!qty[g.id]} onChange={e => setQty(q => ({ ...q, [g.id]: e.target.checked ? 1 : 0 }))} />
                <span className="grow">{g.name}{g.worn && <span className="muted small"> · worn</span>}</span>
                <span className="muted small tnum">{fmtWeight(g.weight_g, units)}</span>
                {!!qty[g.id] && <input type="number" min={1} max={99} className="num-xs" value={qty[g.id]} onChange={e => setQty(q => ({ ...q, [g.id]: Math.max(1, Number(e.target.value)) }))} aria-label={`How many ${g.name}`} />}
              </label>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}

// ---- routes from files ----

export function ImportRoutesModal({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const { settings } = useApp()
  const units = settings?.units || 'metric'
  const { toast } = useToast()
  const [files, setFiles] = useState<File[]>([])
  const [activity, setActivity] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<{ routes: Route[]; problems?: string[] } | null>(null)
  const go = async () => {
    setBusy(true)
    try {
      const r = await uploadMany<{ routes: Route[]; problems?: string[] }>(`${V1}/routes/import${activity ? `?activity=${activity}` : ''}`, files)
      announceChange()
      if (r.routes.length === 1 && !r.problems?.length) { onClose(); navigate(`/routes/${r.routes[0].id}`); return }
      setDone(r)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Bring in a route" onClose={onClose} footer={done ? <button type="button" className="btn primary" onClick={onClose}>Done</button> : <>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={go} disabled={busy || !files.length}>{busy ? <Spinner /> : <Icon name="upload" size={15} />}Bring in {files.length > 1 ? `${files.length} files` : 'the file'}</button>
    </>}>
      {done ? <div className="stack">
        <ul className="list bordered">{done.routes.map(r => (
          <li key={r.id}><button type="button" className="list-row linkrow" onClick={() => { onClose(); navigate(`/routes/${r.id}`) }}>
            <span className="list-ico"><Icon name={ACTIVITY[r.activity].icon} size={15} /></span>
            <span className="row-main grow"><span className="row-title">{r.name}</span><span className="muted small">{fmtDist(r.distance_m, units)} · {fmtHeight(r.gain_m, units)} up · {fmtMinutes(r.minutes)}</span></span>
          </button></li>))}</ul>
        {done.problems?.map(p => <p key={p} className="warn-text small">{p}</p>)}
      </div> : <div className="stack">
        <p className="muted">GPX from Komoot, AllTrails, Strava, Garmin or OsmAnd; KML from Google My Maps; TCX; GeoJSON. Every track becomes a route, with the spots marked in the file. Bringing the same file in twice finds the route you already have.</p>
        <label className="drop-zone">
          <Icon name="upload" size={22} />
          <span>{files.length ? files.map(f => f.name).join(', ') : 'Choose files'}</span>
          <input type="file" multiple accept=".gpx,.kml,.tcx,.geojson,.json,application/gpx+xml" onChange={e => setFiles(Array.from(e.target.files || []))} />
        </label>
        <Field label="What are they" group help="Leave as “From the file” to use what the file says.">
          <Select ariaLabel="Activity" value={activity} onChange={setActivity} options={[{ value: '', label: 'From the file' }, ...ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))]} />
        </Field>
        {settings?.online === false && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> Files without heights stay without a climb: looking heights up online is switched off (Settings → General).</p>}
      </div>}
    </Modal>
  )
}

// ---- a route from a day Pocket Movement recorded ----

export function FromMovementModal({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const { settings } = useApp()
  const units = settings?.units || 'metric'
  const { toast } = useToast()
  const appState = useAppState()
  const movement = appState('movement', 'movement:read')
  const [date, setDate] = useState(() => { const d = new Date(); d.setDate(d.getDate() - 1); return d.toISOString().slice(0, 10) })
  const { data, loading, error } = useAsync(() => movement.ok ? api.get<{ tracks: MovementTrack[]; open_url: string }>(`${V1}/movement/day?date=${date}`) : Promise.resolve(null), [date, movement.ok])
  const [pick, setPick] = useState<MovementTrack | null>(null)
  const [name, setName] = useState('')
  const [activity, setActivity] = useState<Activity>((settings?.default_activity || 'hike'))
  const [busy, setBusy] = useState(false)
  const make = async () => {
    if (!pick) return
    setBusy(true)
    try {
      const r = await api.post<Route>(`${V1}/routes/from-movement`, { start_at: pick.start_at, end_at: pick.end_at, name, activity })
      announceChange(); onClose(); navigate(`/routes/${r.id}`)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="A route from a day you recorded" onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={make} disabled={busy || !pick}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Make it a route</button>
    </>}>
      {!movement.ok ? <p className="muted">{movement.loading ? 'Checking Pocket Movement…' : `Pocket Movement is not connected for you${movement.app?.fix ? `: ${movement.app.fix}` : ''}.`}</p> : <div className="stack">
        <Field label="Day"><input type="date" value={date} max={todayISO()} onChange={e => { setDate(e.target.value); setPick(null) }} /></Field>
        {loading && <Spinner />}
        {error && <p className="warn-text small">{errMsg(error)}</p>}
        {data && !data.tracks.length && <p className="muted">Nothing recorded that day.</p>}
        {data && data.tracks.length > 0 && <ul className="list bordered">{data.tracks.map(t => (
          <li key={t.id}><button type="button" className={`list-row linkrow ${pick?.id === t.id ? 'picked' : ''}`} onClick={() => setPick(t)} aria-pressed={pick?.id === t.id}>
            <span className="list-ico"><Icon name={pick?.id === t.id ? 'check' : 'route'} size={15} /></span>
            <span className="row-main grow"><span className="row-title">{new Date(t.start_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} – {new Date(t.end_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
              <span className="muted small">{fmtDist(t.distance_m, units)} · {fmtMinutes(Math.round(t.duration_s / 60))}{t.mode ? ` · ${t.mode}` : ''}</span></span>
          </button></li>))}</ul>}
        {pick && <div className="two-col tight">
          <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} placeholder={`${ACTIVITY[activity].label} on ${fmtDate(date)}`} /></Field>
          <Field label="What" group><Select ariaLabel="Activity" value={activity} onChange={v => setActivity(v as Activity)} options={ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))} /></Field>
        </div>}
      </div>}
    </Modal>
  )
}

// ---- stars ----

export function Stars({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  return (
    <span className="stars" role="radiogroup" aria-label="How was it">
      {[1, 2, 3, 4, 5].map(n => <button key={n} type="button" role="radio" aria-checked={value === n} aria-label={`${n} of 5`} className={n <= value ? 'on' : ''} onClick={() => onChange(value === n ? 0 : n)}><Icon name="star" size={20} /></button>)}
    </span>
  )
}

export { Segmented }
