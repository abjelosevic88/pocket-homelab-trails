// The height along a route as a profile. Moving over it says the distance,
// the height and how steep it is there, and a dot follows on the map; the
// spots along the way sit on the line.
import { useCallback, useMemo, useRef, useState } from 'react'
import { useApp } from '../lib/app-context'
import { WAYPOINT, cumulative, distUnit, fmtDist, fmtHeight, heightUnit } from '../lib/trails'
import type { Pt, Waypoint } from '../lib/types'
import { iconSVG } from './icons'

const PL = 44, PR = 10, PT = 12, PB = 24

export function Elevation({ track, waypoints, onHover, compact = false }: { track: Pt[]; waypoints?: Waypoint[]; onHover?: (atM: number | null) => void; compact?: boolean }) {
  const { settings } = useApp()
  const units = settings?.units || 'metric'
  const box = useRef<SVGSVGElement | null>(null)
  const [at, setAt] = useState<number | null>(null)
  // drawn at its real size (the CSS sets the height per page), so labels and dots are never stretched
  const [size, setSize] = useState({ w: 800, h: 150 })
  const ro = useRef<ResizeObserver | null>(null)
  const measured = useCallback((el: SVGSVGElement | null) => {
    box.current = el
    ro.current?.disconnect()
    ro.current = null
    if (!el) return
    const measure = () => {
      const r = el.getBoundingClientRect(), w = Math.round(r.width), h = Math.round(r.height)
      if (w > 0 && h > 0) setSize(s => s.w === w && s.h === h ? s : { w, h })
    }
    measure()
    ro.current = new ResizeObserver(measure)
    ro.current.observe(el)
  }, [])
  const W = size.w, H = size.h
  const data = useMemo(() => {
    const cum = cumulative(track)
    const pts: { d: number; e: number }[] = []
    track.forEach((p, i) => { if (p[2] != null) pts.push({ d: cum[i], e: p[2] }) })
    if (pts.length < 2) return null
    // smooth a little (a moving average over ~5 points) so GPS noise does not look like cliffs
    const w = Math.max(2, Math.round(pts.length / 120))
    const sm = pts.map((p, i) => {
      let s = 0, n = 0
      for (let k = Math.max(0, i - w); k <= Math.min(pts.length - 1, i + w); k++) { s += pts[k].e; n++ }
      return { d: p.d, e: s / n }
    })
    const total = cum[cum.length - 1] || 1
    let lo = Math.min(...sm.map(p => p.e)), hi = Math.max(...sm.map(p => p.e))
    const span = Math.max(hi - lo, 60)
    lo = Math.floor((lo - span * 0.08) / 10) * 10
    hi = Math.ceil((hi + span * 0.12) / 10) * 10
    return { sm, total, lo, hi, cum }
  }, [track])
  if (!data) return <div className="elev-none muted small">No heights for this route{settings?.online === false ? ' (looking them up online is switched off in Settings)' : ''}.</div>
  const { sm, total, lo, hi } = data
  const x = (d: number) => PL + (d / total) * (W - PL - PR)
  const y = (e: number) => PT + (1 - (e - lo) / (hi - lo)) * (H - PT - PB)
  const line = sm.map((p, i) => `${i ? 'L' : 'M'}${x(p.d).toFixed(1)},${y(p.e).toFixed(1)}`).join('')
  const area = `${line}L${x(sm[sm.length - 1].d).toFixed(1)},${H - PB}L${x(sm[0].d).toFixed(1)},${H - PB}Z`
  const hu = heightUnit(units), du = distUnit(units)
  const yTicks = niceTicks(lo / hu.f, hi / hu.f, 4).map(v => v * hu.f)
  const xTicks = niceTicks(0, total / du.f, W < 500 ? 4 : 6).map(v => v * du.f).filter(v => v <= total)
  const near = (d: number) => {
    let best = sm[0]
    for (const p of sm) if (Math.abs(p.d - d) < Math.abs(best.d - d)) best = p
    return best
  }
  const grade = (d: number) => {
    const a = near(Math.max(0, d - 100)), b = near(Math.min(total, d + 100))
    return b.d > a.d ? Math.round(((b.e - a.e) / (b.d - a.d)) * 100) : 0
  }
  const move = (clientX: number) => {
    const r = box.current?.getBoundingClientRect()
    if (!r) return
    const px = ((clientX - r.left) / r.width) * W
    const d = Math.max(0, Math.min(total, ((px - PL) / (W - PL - PR)) * total))
    setAt(d)
    onHover?.(d)
  }
  const leave = () => { setAt(null); onHover?.(null) }
  const cur = at != null ? near(at) : null
  return (
    <div className={`elev ${compact ? 'compact' : ''}`}>
      <svg ref={measured} viewBox={`0 0 ${W} ${H}`} className="elev-svg" role="img" aria-label="Height along the route"
        onMouseMove={e => move(e.clientX)} onMouseLeave={leave} onTouchStart={e => move(e.touches[0].clientX)} onTouchMove={e => move(e.touches[0].clientX)} onTouchEnd={leave}>
        {yTicks.map(v => <g key={v}><line x1={PL} x2={W - PR} y1={y(v)} y2={y(v)} className="elev-grid" /><text x={PL - 6} y={y(v) + 4} className="elev-lbl" textAnchor="end">{fmtHeight(v, units).replace(/ (m|ft)$/, '')}</text></g>)}
        {xTicks.map(v => <text key={v} x={x(v)} y={H - 6} className="elev-lbl" textAnchor="middle">{fmtDist(v, units).replace(/ (km|mi|m)$/, '')}</text>)}
        <path d={area} className="elev-area" />
        <path d={line} className="elev-line" vectorEffect="non-scaling-stroke" />
        {(waypoints || []).filter(w => w.at_m > 0 || w.kind !== 'parking').map(w => {
          const p = near(w.at_m)
          return <g key={w.id} transform={`translate(${x(w.at_m)},${y(p.e)})`} className={`elev-wp k-${w.kind}`}><circle r="8" /><g transform="translate(-6,-6)" dangerouslySetInnerHTML={{ __html: iconSVG((WAYPOINT[w.kind] || WAYPOINT.other).icon, 12) }} /></g>
        })}
        {cur && <g><line x1={x(cur.d)} x2={x(cur.d)} y1={PT} y2={H - PB} className="elev-cursor" /><circle cx={x(cur.d)} cy={y(cur.e)} r="4" className="elev-dot" /></g>}
      </svg>
      <div className="elev-read" aria-live="polite">
        {cur ? <><b>{fmtDist(cur.d, units)}</b> · {fmtHeight(cur.e, units)} · {grade(cur.d) > 0 ? '↗' : grade(cur.d) < 0 ? '↘' : '→'} {Math.abs(grade(cur.d))}%</> : <span className="muted">Move along the profile to see where you are</span>}
      </div>
    </div>
  )
}

function niceTicks(lo: number, hi: number, n: number): number[] {
  const span = hi - lo
  if (span <= 0) return [lo]
  const raw = span / n
  const pow = Math.pow(10, Math.floor(Math.log10(raw)))
  const step = [1, 2, 2.5, 5, 10].map(m => m * pow).find(s => s >= raw) || raw
  const out: number[] = []
  for (let v = Math.ceil(lo / step) * step; v <= hi + 1e-9; v += step) out.push(Math.round(v * 1000) / 1000)
  return out
}
