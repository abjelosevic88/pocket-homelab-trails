// The person's totals and app states, shared by every page.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { AppState, Summary } from './types'

const EVENT = 'ptr:changed'

interface DataState {
  summary: Summary | null
  apps: AppState[] | null
  reload: () => Promise<void>
  reloadApps: () => Promise<void>
}
const Ctx = createContext<DataState>({ summary: null, apps: null, reload: async () => {}, reloadApps: async () => {} })
export function useData() { return useContext(Ctx) }

/** Something changed (an outing, a route): lists and counts reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [apps, setApps] = useState<AppState[] | null>(null)
  const reload = useCallback(async () => {
    const sum = await api.get<Summary>(`${V1}/summary`).catch(() => null)
    setSummary(s => sum || s || { planned: 0, out: [], to_rate: 0, routes: 0, year: null, today: '' })
  }, [])
  const reloadApps = useCallback(async () => {
    try { setApps((await api.get<{ apps: AppState[] }>(`${V1}/apps`)).apps) } catch { setApps(a => a || []) }
  }, [])
  useEffect(() => { reload(); reloadApps() }, [reload, reloadApps])
  useOnChange(reload)
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') { reload(); reloadApps() } }, 120000)
    return () => window.clearInterval(t)
  }, [reload, reloadApps])
  return <Ctx.Provider value={{ summary, apps, reload, reloadApps }}>{children}</Ctx.Provider>
}

/** One app's state for this person; `ok` = the scope works. */
export function useAppState() {
  const { apps } = useData()
  return (slug: string, scope?: string) => {
    const a = apps?.find(x => x.slug === slug)
    const ok = !!a && a.via !== '' && (!scope || a.on.includes(scope))
    return { app: a, ok, loading: !apps }
  }
}
