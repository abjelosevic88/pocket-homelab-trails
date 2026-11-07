// Your gear with what it weighs, and the packing lists you start from.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, upload, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useAppState } from '../lib/data'
import { ACTIVITY, GEAR, GEAR_KINDS, fmtWeight } from '../lib/trails'
import type { Gear as GearT, GearKind, Pack } from '../lib/types'
import { GearForm, PackForm } from '../components/Forms'
import { Icon } from '../components/icons'
import { Card, Confirm, Empty, Loading, MenuButton, Modal, Segmented, Spinner, errMsg, useAsync, useToast } from '../components/ui'

export default function Gear() {
  const { settings } = useApp()
  const u = settings?.units || 'metric'
  const { toast } = useToast()
  const appState = useAppState()
  const inventory = appState('inventory', 'items:read')
  const [tab, setTab] = useState<'gear' | 'packs'>('gear')
  const gear = useAsync(() => api.get<GearT[]>(`${V1}/gear`), [])
  const packs = useAsync(() => api.get<Pack[]>(`${V1}/packs`), [])
  const reload = useCallback(() => { gear.reload(); packs.reload() }, [gear.reload, packs.reload]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { window.addEventListener('ptr:gear', reload); return () => window.removeEventListener('ptr:gear', reload) }, [reload])
  const [edit, setEdit] = useState<GearT | 'new' | null>(null)
  const [editPack, setEditPack] = useState<Pack | 'new' | null>(null)
  const [del, setDel] = useState<{ kind: 'gear' | 'pack'; id: number; name: string } | null>(null)
  const [inv, setInv] = useState(false)
  const file = useRef<HTMLInputElement>(null)
  const groups = useMemo(() => GEAR_KINDS.map(k => [k, (gear.data || []).filter(g => g.category === k)] as const).filter(([, l]) => l.length), [gear.data])
  const importCSV = async (f: File) => {
    try { const r = await upload<{ added: number; found: number }>(`${V1}/gear/import`, 'file', f); toast(`${r.added} new, ${r.found} you had · a packing list “${f.name.replace(/\.[^.]+$/, '')}”`, 'ok'); reload() } catch (e) { toast(errMsg(e), 'err') }
  }
  const remove = async () => {
    if (!del) return
    try { await api.del(`${V1}/${del.kind === 'gear' ? 'gear' : 'packs'}/${del.id}`); reload() } catch (e) { toast(errMsg(e), 'err') } finally { setDel(null) }
  }
  const more = () => [
    { label: 'Bring in a LighterPack list (CSV)', icon: 'upload', onSelect: () => file.current?.click() },
    { label: 'From Pocket Inventory', icon: 'package', disabled: !inventory.ok, hint: inventory.ok ? undefined : 'not connected', onSelect: () => setInv(true) },
  ]
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Gear</h1><p className="muted">{gear.data ? `${gear.data.length} things · ${packs.data?.length || 0} packing lists` : ''}</p></div>
        <MenuButton label="Bring gear in" className="btn" align="right" width={280} items={more}><Icon name="upload" size={15} /><span className="hide-sm">Bring in</span></MenuButton>
        <button type="button" className="btn primary" onClick={() => tab === 'gear' ? setEdit('new') : setEditPack('new')}><Icon name="plus" size={15} />{tab === 'gear' ? 'Add gear' : 'New packing list'}</button>
        <input ref={file} type="file" hidden accept=".csv,text/csv" onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) importCSV(f) }} />
      </div>
      <Segmented className="self-start" value={tab} onChange={v => setTab(v as 'gear' | 'packs')} options={[{ value: 'gear', label: 'Your gear' }, { value: 'packs', label: 'Packing lists' }]} />
      {tab === 'gear' ? (!gear.data ? <Loading /> : !gear.data.length ? (
        <Empty title="No gear yet" icon="backpack"><p className="muted">What you take and what it weighs: then a packing list is a few taps, and you see what your pack weighs.</p><button type="button" className="btn" onClick={() => setEdit('new')}><Icon name="plus" size={15} />Add gear</button></Empty>
      ) : groups.map(([k, list]) => (
        <Card key={k} title={<span className="sec-title"><Icon name={GEAR[k].icon} size={15} />{GEAR[k].label}<span className="count">{list.length}</span></span>} actions={<span className="muted small">{fmtWeight(list.reduce((s, g) => s + g.weight_g, 0), u)}</span>} flush>
          <ul className="list">{list.map(g => (
            <li key={g.id} className="list-row">
              <button type="button" className="row-main grow linkish-row" onClick={() => setEdit(g)}>
                <span className="row-title">{g.name}</span>
                <span className="muted small ellipsis">{[g.worn && 'worn', g.consumable && 'used up', g.packs && `on ${g.packs} ${g.packs === 1 ? 'list' : 'lists'}`, g.inventory_ref && 'from Pocket Inventory', g.note].filter(Boolean).join(' · ')}</span>
              </button>
              <span className="small tnum">{fmtWeight(g.weight_g, u)}</span>
              <button type="button" className="btn ghost icon-only sm" aria-label={`Delete ${g.name}`} onClick={() => setDel({ kind: 'gear', id: g.id, name: g.name })}><Icon name="trash" size={14} /></button>
            </li>))}</ul>
        </Card>
      ))) : (!packs.data ? <Loading /> : !packs.data.length ? (
        <Empty title="No packing lists yet" icon="check-square"><p className="muted">“Day hike”, “Winter day”, “Bike day”: an outing starts its list from one.</p><button type="button" className="btn" onClick={() => setEditPack('new')}><Icon name="plus" size={15} />New packing list</button></Empty>
      ) : <div className="pack-grid">{packs.data.map(p => (
        <Card key={p.id} className="pack-card">
          <div className="row tight"><span className="tile-ico"><Icon name="backpack" size={14} /></span><button type="button" className="grow linkish pack-name" onClick={() => setEditPack(p)}>{p.name}</button>
            <button type="button" className="btn ghost icon-only sm" aria-label={`Delete ${p.name}`} onClick={() => setDel({ kind: 'pack', id: p.id, name: p.name })}><Icon name="trash" size={14} /></button></div>
          <div className="muted small">{p.activity ? ACTIVITY[p.activity].label : 'Anything'} · {p.count} things</div>
          <div className="weights">
            <div><span className="stat-l">In the pack</span><b>{fmtWeight(p.weight.total, u)}</b></div>
            <div><span className="stat-l">Base</span><span>{fmtWeight(p.weight.base, u)}</span></div>
            <div><span className="stat-l">Worn</span><span>{fmtWeight(p.weight.worn, u)}</span></div>
          </div>
          <WeightBar pack={p} />
        </Card>
      ))}</div>)}
      {edit && <GearForm gear={edit === 'new' ? undefined : edit} onClose={() => setEdit(null)} onSaved={reload} />}
      {editPack && <PackForm pack={editPack === 'new' ? undefined : editPack} onClose={() => { setEditPack(null); reload() }} />}
      {inv && <InventoryPicker onClose={() => { setInv(false); reload() }} />}
      {del && <Confirm title={`Delete ${del.name}?`} message={del.kind === 'gear' ? 'It leaves your packing lists; outings keep their copy.' : 'Your gear stays.'} confirmLabel="Delete" onCancel={() => setDel(null)} onConfirm={remove} />}
    </div>
  )
}

/** Where a pack's weight goes, by kind. */
function WeightBar({ pack }: { pack: Pack }) {
  const by = new Map<GearKind, number>()
  for (const it of pack.items) if (it.gear && !it.gear.worn) by.set(it.gear.category, (by.get(it.gear.category) || 0) + it.gear.weight_g * it.qty)
  const total = [...by.values()].reduce((a, b) => a + b, 0)
  if (!total) return null
  const parts = [...by.entries()].sort((a, b) => b[1] - a[1])
  return (
    <div className="wbar" aria-label="Weight by kind">
      {parts.map(([k, g], i) => <span key={k} className={`wseg w${i % 6}`} style={{ width: `${(g / total) * 100}%` }} title={`${GEAR[k].label}: ${Math.round((g / total) * 100)}%`} />)}
    </div>
  )
}

function InventoryPicker({ onClose }: { onClose: () => void }) {
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const { data, loading } = useAsync(() => api.get<{ id: string; name: string; brand: string; category: string; have: boolean }[]>(`${V1}/apps/options/inventory?q=${encodeURIComponent(q)}`), [q])
  const [pick, setPick] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try { const r = await api.post<{ added: number }>(`${V1}/gear/from-inventory`, { items: Object.entries(pick).map(([id, name]) => ({ id, name })) }); toast(`${r.added} added to your gear`, 'ok'); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Gear from Pocket Inventory" onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={save} disabled={busy || !Object.keys(pick).length}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Add {Object.keys(pick).length || ''}</button>
    </>}>
      <div className="stack">
        <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find an item" aria-label="Find an item" autoFocus /></div>
        {loading && !data ? <Loading /> : <div className="pick-list tall">{(data || []).map(it => (
          <label key={it.id} className={`check-row pick-row ${it.have ? 'dim' : ''}`}>
            <input type="checkbox" disabled={it.have} checked={it.have || !!pick[it.id]} onChange={e => setPick(p => { const n = { ...p }; if (e.target.checked) n[it.id] = it.name; else delete n[it.id]; return n })} />
            <span className="grow ellipsis">{it.name}{it.brand && <span className="muted"> · {it.brand}</span>}</span><span className="muted tiny">{it.have ? 'in your gear' : it.category}</span>
          </label>))}</div>}
        <p className="muted tiny m0">Weights are not in Pocket Inventory: add them here afterwards.</p>
      </div>
    </Modal>
  )
}
