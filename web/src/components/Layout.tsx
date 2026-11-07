// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { FromMovementModal, GearForm, ImportRoutesModal, OutingForm } from './Forms'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'ptr-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'outing' | 'import' | 'draw' | 'movement' | 'gear'
export type CreateInitial = Record<string, unknown>

/** Start something from anywhere (a page's button), with what is known already. */
export const openCreate = (k: CreateKind, initial?: CreateInitial) => window.dispatchEvent(new CustomEvent('ptr:create', { detail: { kind: k, initial } }))

/** The "new" menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'Plan an outing', icon: 'calendar-plus', hint: 'N', onSelect: () => onCreate('outing') },
    'sep',
    { label: 'Bring in a route (GPX, KML…)', icon: 'upload', hint: 'I', onSelect: () => onCreate('import') },
    { label: 'Draw a route on the map', icon: 'pen', hint: 'D', onSelect: () => onCreate('draw') },
    { label: 'A route from a day you recorded', icon: 'navigation', onSelect: () => onCreate('movement') },
    'sep',
    { label: 'Add gear', icon: 'backpack', hint: 'A', onSelect: () => onCreate('gear') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { summary } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [making, setMaking] = useState<{ kind: CreateKind; initial: CreateInitial } | null>(null)

  const create = (k: CreateKind, initial?: CreateInitial) => {
    closeDrawer()
    if (k === 'draw') { navigate('/routes/draw'); return }
    setMaking({ kind: k, initial: initial || {} })
  }
  const createRef = useRef(create)
  createRef.current = create

  // /?add=outing (the PWA shortcut) opens the dialog
  useEffect(() => {
    const p = new URLSearchParams(loc.search)
    const a = p.get('add')
    if (a === 'outing' || a === 'import' || a === 'gear') { createRef.current(a); navigate(loc.pathname, { replace: true }) }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; initial?: CreateInitial }>).detail; createRef.current(d.kind, d.initial) }
    window.addEventListener('ptr:shortcuts', open)
    window.addEventListener('ptr:create', make)
    return () => { window.removeEventListener('ptr:shortcuts', open); window.removeEventListener('ptr:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('outing'); break
        case 'i': mk('import'); break
        case 'd': mk('draw'); break
        case 'a': mk('gear'); break
        case 'h': go('/'); break
        case 'r': go('/routes'); break
        case 'o': go('/outings'); break
        case 'm': go('/map'); break
        case 'g': go('/gear'); break
        case 'y': go('/year'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'outing', label: 'Plan an outing', icon: 'calendar-plus', hint: 'N', keywords: 'hike walk ride run trip go', run: () => create('outing') },
    { id: 'import', label: 'Bring in a route (GPX, KML, TCX, GeoJSON)', icon: 'upload', hint: 'I', keywords: 'import file komoot alltrails strava garmin', run: () => create('import') },
    { id: 'draw', label: 'Draw a route on the map', icon: 'pen', hint: 'D', keywords: 'plan new route planner', run: () => create('draw') },
    { id: 'movement', label: 'A route from a day you recorded', icon: 'navigation', keywords: 'movement track recorded', run: () => create('movement') },
    { id: 'gear', label: 'Add gear', icon: 'backpack', hint: 'A', keywords: 'pack weight kit', run: () => create('gear') },
    { id: 'today', label: 'Today', icon: 'today', hint: 'H', run: () => navigate('/') },
    { id: 'routes', label: 'Routes', icon: 'route', hint: 'R', run: () => navigate('/routes') },
    { id: 'outings', label: 'Outings', icon: 'calendar', hint: 'O', keywords: 'planned done history', run: () => navigate('/outings') },
    { id: 'map', label: 'Map of your routes', icon: 'map', hint: 'M', run: () => navigate('/map') },
    { id: 'gearp', label: 'Gear and packing lists', icon: 'backpack', hint: 'G', keywords: 'weight lighterpack', run: () => navigate('/gear') },
    { id: 'year', label: 'Your year', icon: 'chart', hint: 'Y', keywords: 'stats totals distance climb', run: () => navigate('/year') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'safety', label: 'Settings: who to tell when you are not back', icon: 'shield', keywords: 'safety emergency contact back by', run: () => navigate('/settings/safety') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'movement weather calendar contacts health mail inventory places travel notes tasks core', run: () => navigate('/settings/apps') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export', icon: 'download', keywords: 'backup json csv', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  const close = () => setMaking(null)
  const m = making
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className={`content ${/^\/(map|routes\/draw|routes\/\d+\/draw)/.test(loc.pathname) ? 'full' : ''}`}><div className="content-inner">{summary ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !/^\/(settings|map|routes\/draw|routes\/\d+\/draw)/.test(loc.pathname) && <button type="button" className="fab" aria-label="Plan an outing" title="Plan an outing" onClick={() => create('outing')}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {m?.kind === 'outing' && <OutingForm initial={m.initial} onClose={close} />}
        {m?.kind === 'import' && <ImportRoutesModal onClose={close} />}
        {m?.kind === 'movement' && <FromMovementModal onClose={close} />}
        {m?.kind === 'gear' && <GearForm onClose={close} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = {
  '': ['Today', '/'], routes: ['Routes', '/routes'], outings: ['Outings', '/outings'], map: ['Map', '/map'], gear: ['Gear', '/gear'], year: ['Your year', '/year'],
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('ptr:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
