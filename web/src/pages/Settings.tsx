import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { ContactSearch } from '../components/Forms'
import { ACTIVITIES, ACTIVITY, APP_INFO, STATE_WORDS, fmtMinutes } from '../lib/trails'
import { upload } from '../lib/api'

type TabId = 'general' | 'safety' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Units, your speed, the map, looking things up online, dates and the app title.' },
  { id: 'safety', label: 'Safety', icon: 'shield', sub: 'Who to tell when you are not back by the time you set, and what goes to your other apps by itself.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'The evening before and the morning of an outing: weather, sunset and what is not packed yet.' },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Movement, weather, calendar, contacts, health, mail, inventory, places, travel, notes and tasks — what your trails can reach.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who helps you: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Each person has their own routes and outings.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and other tools.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Export everything, bring an export back, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'safety' && <SafetyTab />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Routes" icon="route">
        <SettingRow title="Units"><Segmented size="sm" value={f.units} onChange={v => setF({ ...f, units: v })} options={[{ value: 'metric', label: 'Kilometres, metres' }, { value: 'imperial', label: 'Miles, feet' }]} /></SettingRow>
        <SettingRow title="Your speed" help="Route times are the hiking clubs’ usual (4 km an hour on the flat, 300 m an hour up). 120% plans a fifth faster; after an outing with a track, Trails says what you really did.">
          <span className="row tight"><input type="number" className="num-sm" min={40} max={250} step={5} value={f.pace_pct} onChange={e => setF({ ...f, pace_pct: Number(e.target.value) || 100 })} aria-label="Speed in percent" /><span className="muted">%</span></span>
        </SettingRow>
        <SettingRow title="Usually you go" help="New routes and outings start with it."><Select ariaLabel="Activity" value={f.default_activity} onChange={v => setF({ ...f, default_activity: v as SettingsT['default_activity'] })} options={ACTIVITIES.map(a => ({ value: a, label: ACTIVITY[a].label }))} /></SettingRow>
      </Section>
      <Section title="The map" icon="map">
        <SettingRow title="Map" help="Topographic (OpenTopoMap: contour lines, paths, huts) or the street map (OpenStreetMap). The buttons on any map switch it too."><Segmented size="sm" value={f.map_layer} onChange={v => setF({ ...f, map_layer: v })} options={[{ value: 'topo', label: 'Topographic' }, { value: 'osm', label: 'Street map' }]} /></SettingRow>
        <SettingRow title="Marked routes over the map" help="The official hiking and cycling routes (Waymarked Trails)."><Switch checked={f.trail_overlay} onChange={v => setF({ ...f, trail_overlay: v })} label="Marked routes" /></SettingRow>
        <SettingRow title="Look things up online" help="Place search (OpenStreetMap), heights for drawn routes and files without them (Open-Meteo), drawing along paths (BRouter), and the forecast when Pocket Weather is not connected. Free services, no account; only the spots are sent."><Switch checked={f.online} onChange={v => setF({ ...f, online: v })} label="Online lookups" /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function SafetyTab() {
  const { f, setF, save, busy } = useSettingsForm()
  if (!f) return <Loading />
  const on = (slug: AppSlug) => f[`${slug}_mode`] !== 'off' && f[`${slug}_via`] !== ''
  return (
    <div className="stack loose">
      <Section title="Who to tell" icon="shield" sub="When an outing has a back-by time and “tell someone” is on: at that time you get a message; if you have not tapped “I'm back” a while later, this person gets an email with your plan (where you went, the start on a map, who is with you). When you are back, they are told that too.">
        <SettingRow title="Name"><input value={f.contact_name} onChange={e => setF({ ...f, contact_name: e.target.value })} placeholder="Lejla" /></SettingRow>
        <SettingRow title="Email"><input type="email" value={f.contact_email} onChange={e => setF({ ...f, contact_email: e.target.value })} placeholder="name@example.com" /></SettingRow>
        {on('contacts') && <SettingRow title="From Pocket Contacts" stack><ContactSearch onPick={c => setF({ ...f, contact_name: c.name, contact_email: c.email || f.contact_email, contact_ref: c.id })} /></SettingRow>}
        <SettingRow title="Tell them" help="After the back-by time."><Select ariaLabel="Wait" value={String(f.alert_after)} onChange={v => setF({ ...f, alert_after: Number(v) })} options={[0, 15, 30, 60, 90, 120, 180, 240].map(m => ({ value: String(m), label: m ? `${fmtMinutes(m)} late` : 'At once' }))} /></SettingRow>
        <SettingRow title="New outings tell them" help="On for every new outing with a back-by time (each outing can switch it off)."><Switch checked={f.safety_default} onChange={v => setF({ ...f, safety_default: v })} label="Tell someone by default" /></SettingRow>
        {!on('mail') && <div className="callout info"><Icon name="info" size={15} /><div>The emails go through Pocket Mail, from your own mailbox. Connect it under Connected apps; until then you still get the messages yourself.</div></div>}
      </Section>
      <Section title="By itself, in your other apps" icon="link" sub="Each only when its app is connected.">
        <SettingRow title="Outings in Pocket Calendar" help={on('calendar') ? 'From setting off to the back-by time, kept in step when you change them; called off removes them.' : 'Connect Pocket Calendar first.'}><Switch checked={f.calendar_sync} onChange={v => setF({ ...f, calendar_sync: v })} label="Outings in the calendar" disabled={!on('calendar')} /></SettingRow>
        <SettingRow title="Walked track from Pocket Movement" help={on('movement') ? 'When you are back (and again for two days, as phones upload late), the track it recorded is compared with the route.' : 'Connect Pocket Movement first.'}><Switch checked={f.movement_auto} onChange={v => setF({ ...f, movement_auto: v })} label="Take the walked track" disabled={!on('movement')} /></SettingRow>
        <SettingRow title="Finished outings in Pocket Health" help={on('health') ? 'As a workout: hiking, walking, running, cycling or skiing, with distance, climb and time.' : 'Connect Pocket Health first.'}><Switch checked={f.health_log} onChange={v => setF({ ...f, health_log: v })} label="Workouts in Pocket Health" disabled={!on('health')} /></SettingRow>
        <SettingRow title="Send emails from" help={on('mail') ? 'One of your Pocket Mail addresses; empty: its default.' : 'Connect Pocket Mail first.'}><input value={f.mail_from} onChange={e => setF({ ...f, mail_from: e.target.value })} placeholder="you@example.com" disabled={!on('mail')} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const now = async () => { try { const r = await api.post<{ text: string }>(`${V1}/notify/check?send=1`); toast(r.text.split('\n')[0]) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/trails" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to tell you" icon="clock">
        <SettingRow title="The evening before" help="Tomorrow’s outing: the weather at the start, sunset, what to know and what is not packed yet. Nothing on an evening with nothing planned."><Switch checked={f.notify_evening} onChange={v => setF({ ...f, notify_evening: v })} label="The evening before" /></SettingRow>
        <SettingRow title="Evening message at" help="Server time."><Select ariaLabel="Evening hour" value={String(f.evening_hour)} onChange={v => setF({ ...f, evening_hour: Number(v) })} options={HOURS} /></SettingRow>
        <SettingRow title="The morning of" help="The same, on the day."><Switch checked={f.notify_morning} onChange={v => setF({ ...f, notify_morning: v })} label="The morning of" /></SettingRow>
        <SettingRow title="Morning message at" help="Server time."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
        <SettingRow title="“Are you back?”" help="When an outing’s back-by time passes, always (the safety message cannot be switched off while “tell someone” is on)."><span className="muted small">always</span></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        <button type="button" className="btn" onClick={now}><Icon name="refresh" size={15} />Tomorrow’s message now</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'movement', title: 'Pocket Movement — what you really walked', sub: 'Compare the walked track with the plan (distance, time on the move, how much of the route you did), and make a route from a day you recorded.', token: 'Pocket Movement → Settings → API keys', port: '8197' },
  { slug: 'weather', title: 'Pocket Weather — the forecast on the day', sub: 'At the start of the route: rain, wind, heat and frost hour by hour, and sunset. Without it, Trails asks Open-Meteo directly (when looking things up online is on).', token: 'Pocket Weather → Settings → API tokens', port: '8239' },
  { slug: 'calendar', title: 'Pocket Calendar — outings in your calendar', sub: 'From setting off to the back-by time, kept in step; called off removes it.', token: 'Pocket Calendar → Settings → API tokens', port: '8190' },
  { slug: 'contacts', title: 'Pocket Contacts — who comes, who to tell', sub: 'Pick companions and your safety contact from your contacts. Without it, type their names and email.', token: 'Pocket Contacts → Settings → API tokens', port: '8191' },
  { slug: 'health', title: 'Pocket Health — workouts', sub: 'A finished outing as a workout, with distance, climb and time.', token: 'Pocket Health → Settings → API tokens', port: '8199' },
  { slug: 'mail', title: 'Pocket Mail — your plan, and “not back yet”', sub: 'Email your plan to someone before you go, and tell them when you are not back in time — from your own mailbox.', token: 'Pocket Mail → Settings → API tokens', port: '8208' },
  { slug: 'inventory', title: 'Pocket Inventory — your outdoor things', sub: 'Bring the things you keep in Pocket Inventory into your gear list.', token: 'Pocket Inventory → Settings → API tokens', port: '8170' },
  { slug: 'places', title: 'Pocket Places — huts and restaurants on the way', sub: 'Your saved places near a route show on its map; a spot on the way can be saved as a place.', token: 'Pocket Places → Settings → API tokens', port: '8236' },
  { slug: 'travel', title: 'Pocket Travel — trips', sub: 'File an outing under the trip it is part of; the trip page in Pocket Travel can show it.', token: 'Pocket Travel → Settings → API tokens', port: '8174' },
  { slug: 'notes', title: 'Pocket Notes — how it went', sub: 'Save an outing’s account (route, who, the numbers, trail conditions) as a note.', token: 'Pocket Notes → Settings → API tokens', port: '8182' },
  { slug: 'tasks', title: 'Pocket Tasks — gear to buy', sub: 'What you still need to buy for an outing as tasks, due the day before.', token: 'Pocket Tasks → Settings → API tokens', port: '8184' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [f?.movement_mode, f?.weather_mode, f?.calendar_mode, f?.contacts_mode, f?.health_mode, f?.mail_mode, f?.inventory_mode, f?.places_mode, f?.travel_mode, f?.notes_mode, f?.tasks_mode])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Trails only adds things in them when you ask (or when you switched that on under Safety), and everything here works without them. Through <b>Pocket Core</b> Trails acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        const st = states?.find(x => x.slug === a.slug)
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={st && st.state !== 'on' && st.state !== 'off' ? <span className={`badge ${st.state === 'partly' ? 'ok' : 'warn'}`}>{STATE_WORDS[st.state]}</span> : connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {st?.fix && st.state !== 'on' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.fix}.</p>}
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="Trails offers your trails to the rest of the box through Pocket Core: Pocket Day writes your outings into your journal, Pocket Weather adds them to its plans, Pocket Travel shows them on a trip, Pocket Search finds routes and outings, Pocket AI tells you what is next and plans an outing when you ask, and Pocket Flows can start a flow when you are not back in time. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>trails:read</dt><dd className="muted">Your routes (with the GPX), gear and packing lists, planned and finished outings with their weather and how they went, and the year’s totals.</dd></div>
          <div><dt>trails:write</dt><dd className="muted">Plan an outing or add a route for you; that app may change only what it added. It can never arm “tell someone”.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>trails.outing_planned</code>, <code>trails.outing_today</code>, <code>trails.outing_started</code>, <code>trails.outing_done</code>, <code>trails.outing_cancelled</code>, <code>trails.overdue</code>, <code>trails.route_saved</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="Only when you press its button: what else to pack for an outing (from the route, the weather and your own gear), a draft of how an outing went, and a description of a route. Only that outing or route is sent, and nothing is saved until you say so. Questions about all your trails go to Pocket AI through the Ask button at the top.">
        <SettingRow title="Pocket AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Trails does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own routes, outings, gear, settings and connections. Admins manage accounts; they do not see anyone else’s trails.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their routes, outings, payments, repairs and photos." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or send a recording from a Siri Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>. Distances in metres, weights in grams.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /summary</code></dt><dd className="muted">Out now, the next outing, the year’s totals — for Homepage.</dd></div>
          <div><dt><code>POST /outings/&lt;id&gt;/status/back</code></dt><dd className="muted">“I'm back” (from a Shortcut on your phone). <code>/status/start</code> sets off.</dd></div>
          <div><dt><code>POST /outings</code></dt><dd className="muted"><code>{'{"route_id": 3, "date": "2026-11-14", "start_time": "08:30"}'}</code> plans an outing.</dd></div>
          <div><dt><code>POST /routes/import</code></dt><dd className="muted">A GPX, KML, TCX or GeoJSON file as the body (<code>?name=walk.gpx</code>) becomes a route.</dd></div>
          <div><dt><code>GET /routes/&lt;id&gt;/gpx</code></dt><dd className="muted">A route as GPX, for any map app.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">The outings of a day.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const { toast } = useToast()
  const file = React.useRef<HTMLInputElement>(null)
  const bring = async (fl: File) => {
    try { const r = await upload<{ added: Record<string, number> }>(`${V1}/import`, 'file', fl); const a = r.added || {}; toast(`Added ${a.routes || 0} routes, ${a.outings || 0} outings, ${a.gear || 0} gear, ${a.packs || 0} packing lists`) } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="Export" icon="archive" sub="Everything of yours as one JSON file (routes with their tracks and spots, outings, gear and packing lists) — or every outing as one table for a spreadsheet. A single route downloads as GPX on its page.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Outings as CSV</a></div>
      </Section>
      <Section title="Bring an export back" icon="upload" sub="A Pocket Trails export, also from another box. Bringing the same file twice adds nothing twice. Route files (GPX, KML…) come in under Routes → Add a route; a LighterPack list under Gear.">
        <button type="button" className="btn" onClick={() => file.current?.click()}><Icon name="upload" size={15} />Bring a file…</button>
        <input ref={file} type="file" hidden accept=".json,application/json" onChange={e => { const fl = e.target.files?.[0]; e.target.value = ''; if (fl) bring(fl) }} />
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-trails.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Routes', counts.routes, 'route'], ['Outings', counts.outings, 'calendar'], ['Spots', counts.waypoints, 'pin'], ['Gear', counts.gear, 'backpack']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Trails</h3>
          <p className="muted small">Hikes, walks, runs and bike rides: routes with their climb and time, what to pack, the weather on the day, a time to be back by, and how it went — linked to your movement, weather, calendar, contacts, health, mail, inventory, places, travel, notes and tasks. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-trails" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Movement, Weather, Calendar, Contacts, Health, Mail, Inventory, Places, Travel, Notes, Tasks, Day, Flows, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}

