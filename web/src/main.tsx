import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/trails.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Today from './pages/Today'
import RoutesPage from './pages/Routes'
import RoutePage from './pages/RoutePage'
import Planner from './pages/Planner'
import Outings from './pages/Outings'
import OutingPage from './pages/OutingPage'
import MapPage from './pages/MapPage'
import Gear from './pages/Gear'
import Year from './pages/Year'
import Settings from './pages/Settings'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="trails"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Today />} />
        <Route path="/routes" element={<RoutesPage />} />
        <Route path="/routes/draw" element={<Planner />} />
        <Route path="/routes/:id" element={<RoutePage />} />
        <Route path="/routes/:id/draw" element={<Planner />} />
        <Route path="/outings" element={<Outings />} />
        <Route path="/outings/:id" element={<OutingPage />} />
        <Route path="/map" element={<MapPage />} />
        <Route path="/gear" element={<Gear />} />
        <Route path="/year" element={<Year />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
