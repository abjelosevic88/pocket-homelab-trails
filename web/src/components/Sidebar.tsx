// Left column: plan an outing (and the other ways to start), the sections,
// then what is coming up.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { fmtDate } from '../lib/format'
import { ACTIVITY } from '../lib/trails'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { summary } = useData()
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Trails'
  const path = loc.pathname
  const out = summary?.out || []
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number; bad?: boolean; hint?: string }[] = [
    { to: '/', label: 'Today', icon: 'today', active: path === '/', count: out.length || undefined, bad: out.length > 0, hint: out.length ? 'You are out now' : undefined },
    { to: '/routes', label: 'Routes', icon: 'route', active: path.startsWith('/routes') },
    { to: '/outings', label: 'Outings', icon: 'calendar', active: path.startsWith('/outings'), count: summary?.to_rate || undefined, hint: 'Outings waiting for “how did it go?”' },
    { to: '/map', label: 'Map', icon: 'map', active: path === '/map' },
    { to: '/gear', label: 'Gear', icon: 'backpack', active: path.startsWith('/gear') },
    { to: '/year', label: 'Your year', icon: 'chart', active: path === '/year' },
  ]
  const cur = path.startsWith('/outings/') ? Number(path.split('/')[2]) : 0
  return (
    <aside className="sidebar" aria-label="Pocket Trails">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Trails' ? title : <>Pocket <b>Trails</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('outing')} title="Plan an outing (N)">
          <Icon name="plus" size={15} strokeWidth={2.2} />Plan an outing
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={290} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${n.bad ? 'bad' : ''}`} title={n.hint}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {(out.length > 0 || summary?.next) && <section className="sb-sec">
        <h4 className="sb-h">{out.length ? 'Out now' : 'Next'}</h4>
        {out.map(o => (
          <button key={o.id} type="button" className={`sb-row sb-list ${cur === o.id ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/outings/${o.id}`) }}>
            <span className="sb-place-ico out" aria-hidden="true"><Icon name="navigation" size={14} /></span>
            <span className="grow ellipsis">{o.title || o.route_name || ACTIVITY[o.activity].label}</span>
          </button>
        ))}
        {!out.length && summary?.next && <button type="button" className={`sb-row sb-list ${cur === summary.next.id ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/outings/${summary.next!.id}`) }}>
          <span className="sb-place-ico" aria-hidden="true"><Icon name={ACTIVITY[summary.next.activity].icon} size={14} /></span>
          <span className="grow ellipsis">{summary.next.title}</span>
          <span className="muted small">{summary.next.days === 0 ? 'today' : summary.next.days === 1 ? 'tomorrow' : fmtDate(summary.next.date, false)}</span>
        </button>}
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('ptr:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
