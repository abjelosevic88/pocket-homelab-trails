export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'movement' | 'weather' | 'calendar' | 'contacts' | 'health' | 'mail' | 'inventory' | 'places' | 'travel' | 'notes' | 'tasks'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Units = 'metric' | 'imperial'

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  units: Units; pace_pct: number; default_activity: Activity; map_layer: 'topo' | 'osm'; trail_overlay: boolean; online: boolean
  safety_default: boolean; contact_name: string; contact_email: string; contact_ref: string; alert_after: number
  calendar_sync: boolean; health_log: boolean; movement_auto: boolean; mail_from: string; ai_help: boolean
  notify_evening: boolean; evening_hour: number; notify_morning: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type Activity = 'hike' | 'walk' | 'run' | 'bike' | 'gravel' | 'mtb' | 'ski' | 'other'
export type Level = 'easy' | 'moderate' | 'hard' | 'expert'
export type WaypointKind = 'water' | 'hut' | 'food' | 'shelter' | 'camp' | 'summit' | 'viewpoint' | 'lake' | 'parking' | 'transport' | 'junction' | 'danger' | 'photo' | 'other'
export type GearKind = 'pack' | 'shelter' | 'sleep' | 'clothing' | 'footwear' | 'cook' | 'water' | 'food' | 'navigation' | 'safety' | 'first_aid' | 'electronics' | 'bike' | 'snow' | 'other'
export type OutingStatus = 'planned' | 'out' | 'done' | 'cancelled'

/** [lat, lng, ele|null, unix?] */
export type Pt = [number, number, number | null, number?]

export interface Waypoint {
  id: number; route_id: number; kind: WaypointKind; name: string; note: string; lat: number; lng: number; ele: number | null; at_m: number; place_ref: string; created_at: string
}

export interface Route {
  id: number; name: string; activity: Activity; difficulty: '' | Level; region: string; description: string; tags: string[]
  track?: Pt[]; controls?: [number, number][]; drawn: '' | 'straight' | 'paths'
  distance_m: number; gain_m: number; loss_m: number; min_ele: number | null; max_ele: number | null; minutes_set: number; loop: boolean
  start_lat: number | null; start_lng: number | null; end_lat: number | null; end_lng: number | null; start_name: string; link_url: string
  favourite: boolean; archived: boolean; source: string; source_ref: string; created_at: string; updated_at: string
  minutes: number; level: Level | ''; done: number; last_done: string; next?: string; points: number; preview?: [number, number][]; waypoints?: Waypoint[]
}

export interface Person { name: string; email?: string; ref?: string }

export interface OutingItem {
  id: number; outing_id: number; gear_id: number | null; name: string; category: GearKind; weight_g: number; qty: number; worn: boolean; consumable: boolean
  packed: boolean; to_buy: boolean; task_ref: string; position: number
}

export interface DayWeather {
  date: string; code: number; condition: string; text?: string; t_max: number | null; t_min: number | null; precip: number | null; precip_prob: number | null
  wind: number | null; gust: number | null; uv: number | null; sunrise: string; sunset: string
  hours?: { time: string; t: number | null; precip_prob: number | null; code: number; gust: number | null }[]
  elevation?: number; source: string; fetched_at: string
}

export interface Comparison {
  distance_m: number; gain_m: number; total_s: number; moving_s: number; stopped_s: number; from: number; to: number; points: number
  covered_pct: number; max_off_m: number; off_km: number; planned_m: number; planned_min: number; speed_pct: number; has_ele: boolean
}

export interface Outing {
  id: number; route_id: number | null; title: string; activity: Activity; date: string; start_time: string; status: OutingStatus; people: Person[]; notes: string
  back_by: string; safety: boolean; contact_name: string; contact_email: string; contact_ref: string; alert_after: number
  plan_sent_at: string; warned_at: string; alerted_at: string; alert_error: string; started_at: string; ended_at: string
  weather: DayWeather | null; trip_ref: string; trip_name: string; cal_ref: string
  rating: number; conditions: string; report: string; actual_track?: Pt[]; actual: Comparison | null; actual_source: '' | 'movement' | 'gpx'
  health_ref: string; note_ref: string; source: string; source_ref: string; created_at: string; updated_at: string
  route_name?: string; distance_m: number; gain_m: number; minutes: number; items?: OutingItem[]; item_count: number; packed_count: number
}

export interface Advice { level: 'info' | 'caution' | 'warning'; icon: string; text: string }

export interface OutingPage {
  outing: Outing; route: Route | null; advice: Advice[]; sunrise: string; sunset: string; weather_note: string; links: Record<string, string>
  suggest_back_by: string; finish: string; note?: string; message?: string; open_url?: string
}

export interface Gear {
  id: number; name: string; category: GearKind; weight_g: number; worn: boolean; consumable: boolean; note: string; inventory_ref: string; archived: boolean; created_at: string; packs: number
}

export interface Weights { base: number; worn: number; consumable: number; total: number }

export interface Pack {
  id: number; name: string; activity: '' | Activity; note: string; created_at: string; items: { gear_id: number; qty: number; gear?: Gear }[]; count: number; weight: Weights
}

export interface ActivityTotal { outings: number; distance_m: number; gain_m: number; minutes: number }

export interface YearTotals {
  year: number; outings: number; distance_m: number; gain_m: number; minutes: number; by_activity: Record<string, ActivityTotal>
  by_month: { month: number; outings: number; distance_m: number; gain_m: number }[]; summits: number; highest: number | null; longest?: Outing
}

export interface Summary {
  planned: number; out: Outing[]; to_rate: number; routes: number; year: YearTotals | null; today: string
  next?: { id: number; title: string; date: string; start_time: string; activity: Activity; days: number }
}

export interface TodayView {
  today: string; current: OutingPage | null; upcoming: Outing[]; recent: Outing[]; unmarked: Outing[]; ideas: Route[]; year: YearTotals; routes: number
}

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState {
  slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string
}

export interface ContactHit { id: string; name: string; email: string; phone: string; initials: string; color: string }
export interface Trip { id: string; name: string; destination: string; start_date: string; end_date: string; during: boolean; url: string }
export interface NearPlace { id: string; name: string; kind: string; status: string; rating: number | null; lat: number; lng: number; at_m: number; off_m: number; url: string }
export interface FoundPlace { name: string; detail: string; lat: number; lng: number; kind: string; ele?: number; bbox?: number[] }
export interface DrawResult { track: Pt[]; distance_m: number; gain_m: number; loss_m: number; min_ele: number | null; max_ele: number | null; loop: boolean; minutes: number; level: Level; heights_note: string }
export interface MovementTrack { id: string; start_at: string; end_at: string; distance_m: number; duration_s: number; mode: string; gain_m: number; preview: [number, number][] }
