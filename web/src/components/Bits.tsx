// Pieces the pages share: a route or an outing in a list, the weather of a
// day, what to know before going, a thin line of a route.
import { Link } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { fmtDate } from '../lib/format'
import { ACTIVITY, ACTIVITY_COLOR, LEVEL, STATUS, fmtDist, fmtHeight, fmtMinutes, fmtTemp, outingTitle, stars, weatherIcon } from '../lib/trails'
import type { Advice, DayWeather, Outing, Route } from '../lib/types'
import { Icon } from './icons'

export function Pill({ cls, icon, children, title }: { cls: string; icon?: string; children: React.ReactNode; title?: string }) {
  return <span className={`pill ${cls}`} title={title}>{icon && <Icon name={icon} size={11} />}{children}</span>
}

/** A route's line drawn small (lists and cards), north up. */
export function RouteShape({ line, activity, size = 44 }: { line?: [number, number][]; activity: Route['activity']; size?: number }) {
  if (!line || line.length < 2) return <span className="route-shape empty" style={{ width: size, height: size }}><Icon name={ACTIVITY[activity].icon} size={16} /></span>
  const lats = line.map(p => p[0]), lngs = line.map(p => p[1])
  const minLat = Math.min(...lats), maxLat = Math.max(...lats), minLng = Math.min(...lngs), maxLng = Math.max(...lngs)
  const k = Math.cos(((minLat + maxLat) / 2) * Math.PI / 180)
  const w = (maxLng - minLng) * k || 1e-6, h = maxLat - minLat || 1e-6
  const s = (size - 8) / Math.max(w, h)
  const ox = (size - w * s) / 2, oy = (size - h * s) / 2
  const d = line.map((p, i) => `${i ? 'L' : 'M'}${(ox + (p[1] - minLng) * k * s).toFixed(1)},${(oy + (maxLat - p[0]) * s).toFixed(1)}`).join('')
  return (
    <svg className="route-shape" width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden="true">
      <path d={d} fill="none" stroke={ACTIVITY_COLOR[activity]} strokeWidth="2.2" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  )
}

export function LevelPill({ level }: { level: Route['level'] }) {
  if (!level) return null
  return <span className={`pill lv ${LEVEL[level].cls}`}>{LEVEL[level].label}</span>
}

export function RouteRow({ r, extra }: { r: Route; extra?: React.ReactNode }) {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  return (
    <Link to={`/routes/${r.id}`} className="list-row route-row">
      <RouteShape line={r.preview} activity={r.activity} />
      <span className="row-main grow">
        <span className="row-title">{r.favourite && <Icon name="star" size={13} className="fav" />}{r.name}</span>
        <span className="muted small ellipsis">{ACTIVITY[r.activity].label} · {fmtDist(r.distance_m, u)} · {fmtHeight(r.gain_m, u)} up · {fmtMinutes(r.minutes)}{r.region ? ` · ${r.region}` : ''}</span>
      </span>
      <span className="row-side">
        <LevelPill level={r.level} />
        {extra ?? (r.next ? <span className="muted tiny">planned {fmtDate(r.next, false)}</span> : r.done ? <span className="muted tiny">done {r.done}×</span> : null)}
      </span>
    </Link>
  )
}

export function OutingRow({ o, right }: { o: Outing; right?: React.ReactNode }) {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const st = STATUS[o.status]
  const dist = o.actual?.distance_m || o.distance_m
  return (
    <Link to={`/outings/${o.id}`} className="list-row">
      <span className={`list-ico ${st.cls}`}><Icon name={o.status === 'done' || o.status === 'planned' ? ACTIVITY[o.activity].icon : st.icon} size={15} /></span>
      <span className="row-main grow">
        <span className="row-title">{outingTitle(o)}</span>
        <span className="muted small ellipsis">{fmtDate(o.date)}{o.start_time ? `, ${o.start_time}` : ''}{dist ? ` · ${fmtDist(dist, u)}` : ''}{o.people.length ? ` · with ${o.people.map(p => p.name.split(' ')[0]).join(', ')}` : ''}</span>
      </span>
      {right ?? (o.status === 'done' ? (o.rating ? <span className="stars-s" aria-label={`${o.rating} of 5`}>{stars(o.rating)}</span> : <Pill cls="st-due" icon="star">Rate it</Pill>) : <Pill cls={st.cls} icon={st.icon}>{st.label}</Pill>)}
    </Link>
  )
}

/** The day's weather in one line (or a few chips). */
export function WeatherLine({ w, sunset, compact = false }: { w: DayWeather | null; sunset?: string; compact?: boolean }) {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  if (!w) return sunset ? <span className="wx-line"><Icon name="sunset" size={14} />Sunset {sunset}</span> : null
  return (
    <span className={`wx-line ${compact ? 'compact' : ''}`}>
      <span className="wx-main"><Icon name={weatherIcon(w.code)} size={compact ? 15 : 18} />{w.condition || w.text}</span>
      {w.t_max != null && <span>{fmtTemp(w.t_min, u)} – {fmtTemp(w.t_max, u)}</span>}
      {w.precip_prob != null && w.precip_prob >= 10 && <span><Icon name="droplet" size={13} />{Math.round(w.precip_prob)}%</span>}
      {w.gust != null && w.gust >= 30 && <span><Icon name="wind" size={13} />{Math.round(w.gust)} km/h</span>}
      {(sunset || w.sunset) && <span><Icon name="sunset" size={13} />{sunset || w.sunset}</span>}
    </span>
  )
}

const ADVICE_ICON: Record<string, string> = { sunset: 'sunset', sunrise: 'sunrise', storm: 'zap', rain: 'cloud-rain', wind: 'wind', heat: 'thermometer', summit: 'mountain', cold: 'snowflake', sun: 'sun', water: 'droplet' }

export function AdviceList({ items }: { items: Advice[] }) {
  if (!items.length) return null
  return (
    <ul className="advice">
      {items.map((a, i) => <li key={i} className={`adv-${a.level}`}><Icon name={ADVICE_ICON[a.icon] || 'info'} size={15} /><span>{a.text}</span></li>)}
    </ul>
  )
}

/** A thin bar of how much is packed. */
export function PackedBar({ packed, total }: { packed: number; total: number }) {
  if (!total) return null
  return <span className="packed-bar" title={`${packed} of ${total} packed`}><span style={{ width: `${(packed / total) * 100}%` }} /></span>
}
