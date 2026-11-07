// Routes on a map (Leaflet): a topographic map with the marked hiking or
// cycling routes over it, or the plain OpenStreetMap. One route in detail
// (its line, the spots along it, the walked track, a dot following the
// elevation profile), many routes as thin lines, or drawing a new one.
// Tapping selects; the page shows the details, never a popup over the map.
import { useEffect, useRef, useState } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { ACTIVITY_COLOR, WAYPOINT, cumulative } from '../lib/trails'
import type { Activity, NearPlace, Pt, Waypoint } from '../lib/types'
import { Icon, iconSVG } from './icons'

const LAYERS = {
  topo: { url: 'https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', attr: 'Map data &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>, SRTM · style &copy; <a href="https://opentopomap.org">OpenTopoMap</a> (CC-BY-SA)', max: 17 },
  osm: { url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png', attr: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>', max: 19 },
}
const DARK = {
  url: 'https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Base/MapServer/tile/{z}/{y}/{x}',
  labels: 'https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Reference/MapServer/tile/{z}/{y}/{x}',
  attr: 'Tiles &copy; Esri', max: 16,
}
const TRAILS = { hiking: 'https://tile.waymarkedtrails.org/hiking/{z}/{x}/{y}.png', cycling: 'https://tile.waymarkedtrails.org/cycling/{z}/{x}/{y}.png' }
const WMT = '&copy; <a href="https://waymarkedtrails.org">Waymarked Trails</a>'

export type MapLine = { id: number; line: [number, number][]; activity: Activity; name: string }

const cycling = (a?: Activity) => a === 'bike' || a === 'gravel' || a === 'mtb'

export function TrailMap({ track, actual, waypoints, places, lines, selected, onSelect, hoverAt, onWaypoint, pickWaypoint, onPick, draw, onDraw, activity, fitKey = '', className = '', me, view }: {
  track?: Pt[] | null; actual?: Pt[] | null; waypoints?: Waypoint[]; places?: NearPlace[]
  lines?: MapLine[]; selected?: number | null; onSelect?: (id: number | null) => void
  hoverAt?: number | null; onWaypoint?: (w: Waypoint) => void
  pickWaypoint?: boolean; onPick?: (lat: number, lng: number) => void
  draw?: [number, number][]; onDraw?: (c: [number, number][]) => void
  activity?: Activity; fitKey?: string; className?: string; me?: [number, number] | null
  view?: { lat: number; lng: number; zoom: number; key: string } | null
}) {
  const ref = useRef<HTMLDivElement>(null)
  const map = useRef<L.Map | null>(null)
  const groups = useRef<Record<string, L.LayerGroup>>({})
  const tiles = useRef<L.Layer[]>([])
  const hoverDot = useRef<L.CircleMarker | null>(null)
  const lastFit = useRef<string | null>(null)
  const cb = useRef({ onSelect, onWaypoint, onPick, onDraw, draw, pickWaypoint })
  cb.current = { onSelect, onWaypoint, onPick, onDraw, draw, pickWaypoint }
  const { theme, settings, setSettings } = useApp()
  const [layer, setLayer] = useState<'topo' | 'osm'>(settings?.map_layer || 'topo')
  const [overlay, setOverlay] = useState(settings?.trail_overlay ?? true)
  const cum = useRef<number[]>([])

  // ---- create once ----
  useEffect(() => {
    if (!ref.current || map.current) return
    const m = L.map(ref.current, { zoomControl: false, attributionControl: true, worldCopyJump: true })
    L.control.zoom({ position: 'bottomright' }).addTo(m)
    m.attributionControl.setPrefix(false)
    L.control.scale({ position: 'bottomleft', imperial: settings?.units === 'imperial', metric: settings?.units !== 'imperial' }).addTo(m)
    m.setView([43.8563, 18.4131], 11)
    for (const k of ['lines', 'actual', 'track', 'wps', 'places', 'draw']) groups.current[k] = L.layerGroup().addTo(m)
    m.on('click', e => {
      const c = cb.current
      if (c.onDraw && c.draw) { c.onDraw([...c.draw, [round(e.latlng.lat), round(e.latlng.lng)]]); return }
      if (c.pickWaypoint && c.onPick) { c.onPick(e.latlng.lat, e.latlng.lng); return }
      c.onSelect?.(null)
    })
    map.current = m
    const ro = new ResizeObserver(() => m.invalidateSize())
    ro.observe(ref.current)
    return () => { ro.disconnect(); m.remove(); map.current = null }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // ---- tiles: the layer, the theme, the marked routes ----
  const trailKind = cycling(activity) ? 'cycling' : 'hiking'
  useEffect(() => {
    const m = map.current
    if (!m) return
    tiles.current.forEach(t => m.removeLayer(t))
    const base = layer === 'osm' && theme === 'dark'
      ? [L.tileLayer(DARK.url, { attribution: DARK.attr, maxNativeZoom: DARK.max, maxZoom: 19 }), L.tileLayer(DARK.labels, { maxNativeZoom: DARK.max, maxZoom: 19 })]
      : [L.tileLayer(LAYERS[layer].url, { attribution: LAYERS[layer].attr, maxNativeZoom: LAYERS[layer].max, maxZoom: 19, subdomains: 'abc' })]
    tiles.current = overlay ? [...base, L.tileLayer(TRAILS[trailKind], { attribution: WMT, maxNativeZoom: 17, maxZoom: 19, opacity: 0.85 })] : base
    tiles.current.forEach(t => t.addTo(m))
  }, [layer, overlay, theme, trailKind])

  // ---- many routes as thin lines ----
  useEffect(() => {
    const g = groups.current.lines
    if (!g) return
    g.clearLayers()
    for (const r of lines || []) {
      if (r.line.length < 2) continue
      const sel = selected === r.id
      const color = ACTIVITY_COLOR[r.activity] || '#2e6b4a'
      if (sel) L.polyline(r.line, { color: '#fff', weight: 8, opacity: 0.9, interactive: false }).addTo(g)
      const pl = L.polyline(r.line, { color, weight: sel ? 5 : 3.5, opacity: selected && !sel ? 0.55 : 0.95 })
      pl.on('click', e => { L.DomEvent.stopPropagation(e); cb.current.onSelect?.(r.id) })
      pl.bindTooltip(r.name, { sticky: true, direction: 'top', className: 'map-tip' })
      g.addLayer(pl)
      const start = r.line[0]
      const pin = L.circleMarker(start, { radius: sel ? 6 : 4.5, color: '#fff', weight: 2, fillColor: color, fillOpacity: 1 })
      pin.on('click', e => { L.DomEvent.stopPropagation(e); cb.current.onSelect?.(r.id) })
      g.addLayer(pin)
    }
  }, [lines, selected])

  // ---- one route in detail ----
  useEffect(() => {
    const g = groups.current.track
    if (!g) return
    g.clearLayers()
    cum.current = track ? cumulative(track) : []
    if (!track || track.length < 2) return
    const ll = track.map(p => [p[0], p[1]] as [number, number])
    const color = ACTIVITY_COLOR[activity || 'hike']
    L.polyline(ll, { color: '#fff', weight: 8, opacity: 0.85, interactive: false }).addTo(g)
    L.polyline(ll, { color, weight: 4.5, opacity: 1, interactive: false, dashArray: actual?.length ? '8 7' : undefined }).addTo(g)
    const end = (p: [number, number], cls: string, title: string) => L.marker(p, { icon: L.divIcon({ className: 'pin-wrap', html: `<div class="end-dot ${cls}" title="${title}"></div>`, iconSize: [16, 16], iconAnchor: [8, 8] }), interactive: false, keyboard: false }).addTo(g)
    const a = ll[0], b = ll[ll.length - 1]
    const loop = Math.abs(a[0] - b[0]) + Math.abs(a[1] - b[1]) < 0.0015
    if (!loop) end(b, 'finish', 'Finish')
    end(a, 'start', 'Start')
  }, [track, activity, actual])

  // ---- what was walked ----
  useEffect(() => {
    const g = groups.current.actual
    if (!g) return
    g.clearLayers()
    if (!actual || actual.length < 2) return
    const ll = actual.map(p => [p[0], p[1]] as [number, number])
    L.polyline(ll, { color: '#fff', weight: 7, opacity: 0.7, interactive: false }).addTo(g)
    L.polyline(ll, { color: '#e8590c', weight: 3.5, opacity: 0.95, interactive: false }).addTo(g)
  }, [actual])

  // ---- spots along the way ----
  useEffect(() => {
    const g = groups.current.wps
    if (!g) return
    g.clearLayers()
    for (const w of waypoints || []) {
      const k = WAYPOINT[w.kind] || WAYPOINT.other
      const icon = L.divIcon({ className: 'pin-wrap', html: `<div class="wp-pin k-${w.kind}">${iconSVG(k.icon, 12)}</div>`, iconSize: [24, 24], iconAnchor: [12, 12] })
      const mk = L.marker([w.lat, w.lng], { icon, title: w.name || k.label, keyboard: true })
      mk.on('click', e => { L.DomEvent.stopPropagation(e); cb.current.onWaypoint?.(w) })
      g.addLayer(mk)
    }
  }, [waypoints])

  // ---- your saved places nearby ----
  useEffect(() => {
    const g = groups.current.places
    if (!g) return
    g.clearLayers()
    for (const p of places || []) {
      const icon = L.divIcon({ className: 'pin-wrap', html: `<div class="place-pin">${iconSVG('pin', 11)}</div>`, iconSize: [20, 20], iconAnchor: [10, 10] })
      L.marker([p.lat, p.lng], { icon, title: p.name, keyboard: false }).bindTooltip(p.name, { direction: 'top', className: 'map-tip' }).addTo(g)
    }
  }, [places])

  // ---- drawing: the points and the straight lines between them ----
  useEffect(() => {
    const g = groups.current.draw
    if (!g) return
    g.clearLayers()
    if (!draw) return
    draw.forEach((c, i) => {
      const icon = L.divIcon({ className: 'pin-wrap', html: `<div class="draw-pt ${i === 0 ? 'first' : ''}">${i + 1}</div>`, iconSize: [22, 22], iconAnchor: [11, 11] })
      const mk = L.marker(c, { icon, draggable: true, title: 'Drag to move · tap to remove' })
      mk.on('dragend', () => { const p = mk.getLatLng(); const next = [...(cb.current.draw || [])]; next[i] = [round(p.lat), round(p.lng)]; cb.current.onDraw?.(next) })
      mk.on('click', e => { L.DomEvent.stopPropagation(e); const next = [...(cb.current.draw || [])]; next.splice(i, 1); cb.current.onDraw?.(next) })
      g.addLayer(mk)
    })
  }, [draw])

  // ---- the dot following the elevation profile ----
  useEffect(() => {
    const m = map.current
    if (!m) return
    if (hoverAt == null || !track || track.length < 2) { hoverDot.current?.remove(); hoverDot.current = null; return }
    const c = cum.current
    let i = c.findIndex(v => v >= hoverAt)
    if (i < 0) i = c.length - 1
    const p: [number, number] = [track[i][0], track[i][1]]
    if (!hoverDot.current) hoverDot.current = L.circleMarker(p, { radius: 7, color: '#fff', weight: 3, fillColor: '#111', fillOpacity: 1, interactive: false }).addTo(m)
    else hoverDot.current.setLatLng(p)
  }, [hoverAt, track])

  // ---- fit to what is shown when it changes (not on every reload) ----
  useEffect(() => {
    const m = map.current
    if (!m) return
    // while drawing only the page's key refits (the map must not jump at every tap)
    const key = draw ? fitKey + '|draw' : fitKey + '|' + (track?.length || 0) + '|' + (lines?.length || 0)
    if (lastFit.current === key) return
    let pts: [number, number][] = []
    if (draw?.length) pts = draw
    else if (track?.length) pts = track.map(p => [p[0], p[1]])
    else if (lines?.length) pts = lines.flatMap(l => l.line)
    if (actual?.length) pts = pts.concat(actual.map(p => [p[0], p[1]] as [number, number]))
    if (!pts.length) { if (me) m.setView(me, 13); lastFit.current = key; return }
    lastFit.current = key
    if (pts.length === 1) m.setView(pts[0], 14)
    else m.fitBounds(L.latLngBounds(pts), { padding: [30, 30], maxZoom: 15 })
  }, [track, lines, fitKey, draw, actual, me])

  // ---- a place searched for ----
  useEffect(() => {
    const m = map.current
    if (m && view) m.setView([view.lat, view.lng], view.zoom)
  }, [view?.key]) // eslint-disable-line react-hooks/exhaustive-deps

  const pickLayer = (l: 'topo' | 'osm') => { setLayer(l); api.put(`${V1}/settings`, { map_layer: l }).then(s => setSettings?.(s as never)).catch(() => {}) }
  const pickOverlay = (v: boolean) => { setOverlay(v); api.put(`${V1}/settings`, { trail_overlay: v }).then(s => setSettings?.(s as never)).catch(() => {}) }

  return (
    <div className={`pmap ${theme === 'dark' && layer === 'topo' ? 'dim' : ''} ${draw || pickWaypoint ? 'picking' : ''} ${className}`}>
      <div ref={ref} style={{ width: '100%', height: '100%' }} />
      <div className="map-layers" role="group" aria-label="Map">
        <button type="button" className={layer === 'topo' ? 'on' : ''} aria-pressed={layer === 'topo'} onClick={() => pickLayer('topo')} title="Topographic map with contour lines">Topo</button>
        <button type="button" className={layer === 'osm' ? 'on' : ''} aria-pressed={layer === 'osm'} onClick={() => pickLayer('osm')} title="Street map">Street</button>
        <button type="button" className={overlay ? 'on' : ''} aria-pressed={overlay} onClick={() => pickOverlay(!overlay)} title={`Marked ${trailKind} routes (Waymarked Trails)`}><Icon name="route" size={13} />Trails</button>
      </div>
    </div>
  )
}

const round = (v: number) => Math.round(v * 1e6) / 1e6
