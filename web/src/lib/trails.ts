// Words, icons and units for routes, outings and gear.
import type { Activity, AppSlug, AppStateName, GearKind, Level, OutingStatus, Units, WaypointKind } from './types'

export const ACTIVITY: Record<Activity, { label: string; noun: string; icon: string }> = {
  hike: { label: 'Hike', noun: 'hike', icon: 'mountain' },
  walk: { label: 'Walk', noun: 'walk', icon: 'footprints' },
  run: { label: 'Run', noun: 'run', icon: 'run' },
  bike: { label: 'Road ride', noun: 'ride', icon: 'bike' },
  gravel: { label: 'Gravel ride', noun: 'ride', icon: 'bike' },
  mtb: { label: 'Mountain bike', noun: 'ride', icon: 'bike' },
  ski: { label: 'Ski tour', noun: 'ski tour', icon: 'snowflake' },
  other: { label: 'Other', noun: 'outing', icon: 'compass' },
}
export const ACTIVITIES = Object.keys(ACTIVITY) as Activity[]

export const LEVEL: Record<Level, { label: string; cls: string }> = {
  easy: { label: 'Easy', cls: 'lv-easy' },
  moderate: { label: 'Moderate', cls: 'lv-moderate' },
  hard: { label: 'Hard', cls: 'lv-hard' },
  expert: { label: 'Very hard', cls: 'lv-expert' },
}

export const WAYPOINT: Record<WaypointKind, { label: string; icon: string }> = {
  water: { label: 'Water', icon: 'droplet' },
  hut: { label: 'Hut', icon: 'home' },
  food: { label: 'Food', icon: 'utensils' },
  shelter: { label: 'Shelter', icon: 'shield' },
  camp: { label: 'Camp', icon: 'tent' },
  summit: { label: 'Summit', icon: 'flag' },
  viewpoint: { label: 'Viewpoint', icon: 'eye' },
  lake: { label: 'Lake', icon: 'waves' },
  parking: { label: 'Parking', icon: 'parking' },
  transport: { label: 'Bus or train', icon: 'bus' },
  junction: { label: 'Junction', icon: 'navigation' },
  danger: { label: 'Take care', icon: 'alert' },
  photo: { label: 'Photo spot', icon: 'camera' },
  other: { label: 'Spot', icon: 'pin' },
}
export const WAYPOINT_KINDS = Object.keys(WAYPOINT) as WaypointKind[]

export const GEAR: Record<GearKind, { label: string; icon: string }> = {
  pack: { label: 'Packs', icon: 'backpack' },
  shelter: { label: 'Shelter', icon: 'tent' },
  sleep: { label: 'Sleep', icon: 'bed' },
  clothing: { label: 'Clothing', icon: 'shield' },
  footwear: { label: 'Footwear', icon: 'footprints' },
  cook: { label: 'Cooking', icon: 'flame' },
  water: { label: 'Water', icon: 'droplet' },
  food: { label: 'Food', icon: 'utensils' },
  navigation: { label: 'Navigation', icon: 'compass' },
  safety: { label: 'Safety', icon: 'alert' },
  first_aid: { label: 'First aid', icon: 'heart-pulse' },
  electronics: { label: 'Electronics', icon: 'smartphone' },
  bike: { label: 'Bike', icon: 'bike' },
  snow: { label: 'Snow and ice', icon: 'snowflake' },
  other: { label: 'Other', icon: 'package' },
}
export const GEAR_KINDS = Object.keys(GEAR) as GearKind[]

export const STATUS: Record<OutingStatus, { label: string; icon: string; cls: string }> = {
  planned: { label: 'Planned', icon: 'calendar', cls: 'st-ok' },
  out: { label: 'Out now', icon: 'navigation', cls: 'st-due' },
  done: { label: 'Done', icon: 'check-circle', cls: 'st-done' },
  cancelled: { label: 'Called off', icon: 'x-circle', cls: 'st-paused' },
}

export const APP_INFO: Record<AppSlug, { name: string; icon: string }> = {
  movement: { name: 'Pocket Movement', icon: 'navigation' },
  weather: { name: 'Pocket Weather', icon: 'cloud' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar' },
  contacts: { name: 'Pocket Contacts', icon: 'users' },
  health: { name: 'Pocket Health', icon: 'heart-pulse' },
  mail: { name: 'Pocket Mail', icon: 'mail' },
  inventory: { name: 'Pocket Inventory', icon: 'package' },
  places: { name: 'Pocket Places', icon: 'pin' },
  travel: { name: 'Pocket Travel', icon: 'plane' },
  notes: { name: 'Pocket Notes', icon: 'note' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square' },
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'connected', partly: 'partly connected', off: 'switched off here', link_off: 'not switched on in Pocket Core', missing: 'not on this box',
  old: 'needs a newer version', no_core: 'no Pocket Core', not_set_up: 'not set up', error: 'not reachable',
}

// ---- units ----

export function fmtDist(m: number | null | undefined, u: Units = 'metric'): string {
  if (m == null) return '—'
  if (u === 'imperial') { const mi = m / 1609.344; return `${mi < 10 ? mi.toFixed(1) : Math.round(mi)} mi` }
  const km = m / 1000
  if (km < 1) return `${Math.round(m)} m`
  return `${km < 10 ? km.toFixed(1) : Math.round(km)} km`
}
export function fmtHeight(m: number | null | undefined, u: Units = 'metric'): string {
  if (m == null) return '—'
  return u === 'imperial' ? `${Math.round(m * 3.28084).toLocaleString()} ft` : `${Math.round(m).toLocaleString()} m`
}
export function fmtWeight(g: number | null | undefined, u: Units = 'metric'): string {
  if (g == null) return '—'
  if (u === 'imperial') { const oz = g / 28.3495; return oz >= 16 ? `${(oz / 16).toFixed(1)} lb` : `${oz.toFixed(1)} oz` }
  return g >= 1000 ? `${(g / 1000).toFixed(g >= 10000 ? 1 : 2)} kg` : `${g} g`
}
export function fmtMinutes(m: number | null | undefined): string {
  if (!m || m <= 0) return '—'
  if (m < 60) return `${m} min`
  const h = Math.floor(m / 60), r = m % 60
  return r ? `${h} h ${String(r).padStart(2, '0')}` : `${h} h`
}
export function fmtSeconds(s: number): string { return fmtMinutes(Math.round(s / 60)) }
export function fmtTemp(t: number | null | undefined, u: Units = 'metric'): string {
  if (t == null) return '—'
  return u === 'imperial' ? `${Math.round(t * 9 / 5 + 32)}°F` : `${Math.round(t)}°`
}
/** A distance unit's name and the factor from metres. */
export function distUnit(u: Units): { label: string; f: number } { return u === 'imperial' ? { label: 'mi', f: 1609.344 } : { label: 'km', f: 1000 } }
export function heightUnit(u: Units): { label: string; f: number } { return u === 'imperial' ? { label: 'ft', f: 1 / 3.28084 } : { label: 'm', f: 1 } }

/** Distance in metres between two points. */
export function dist(a: [number, number], b: [number, number]): number {
  const R = 6371008.8, r = Math.PI / 180
  const dLat = (b[0] - a[0]) * r, dLng = (b[1] - a[1]) * r
  const x = Math.sin(dLat / 2) ** 2 + Math.cos(a[0] * r) * Math.cos(b[0] * r) * Math.sin(dLng / 2) ** 2
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(x)))
}

/** Cumulative distances along a track. */
export function cumulative(t: { 0: number; 1: number }[]): number[] {
  const out = [0]
  for (let i = 1; i < t.length; i++) out.push(out[i - 1] + dist([t[i - 1][0], t[i - 1][1]], [t[i][0], t[i][1]]))
  return out
}

export function weatherIcon(code: number): string {
  if (code >= 95) return 'zap'
  if (code >= 71 && code <= 86 && code !== 80 && code !== 81 && code !== 82) return 'snowflake'
  if (code >= 51) return 'cloud-rain'
  if (code === 45 || code === 48) return 'cloud'
  if (code >= 2) return 'cloud'
  return 'sun'
}

export function stars(n: number): string { return '★'.repeat(n) + '☆'.repeat(Math.max(0, 5 - n)) }

/** "Hike: Trebević" when the outing has no title. */
export function outingTitle(o: { title: string; activity: Activity; route_name?: string }): string {
  return o.title || (o.route_name ? `${ACTIVITY[o.activity].label}: ${o.route_name}` : ACTIVITY[o.activity].label)
}

/** The colour of an activity's line on the map. */
export const ACTIVITY_COLOR: Record<Activity, string> = {
  hike: '#2e6b4a', walk: '#0e7490', run: '#c2410c', bike: '#2563eb', gravel: '#8a6400', mtb: '#7c3aed', ski: '#0891b2', other: '#64748b',
}
