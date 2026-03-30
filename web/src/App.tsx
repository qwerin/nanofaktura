import { BrowserRouter, Routes, Route, NavLink, Navigate } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { ToastContainer } from './components/Toast'
import { AuthProvider, useAuth } from './context/AuthContext'
import { api } from './api/client'
import { InvoiceList } from './pages/InvoiceList'
import { InvoiceNew } from './pages/InvoiceNew'
import { InvoiceDetail } from './pages/InvoiceDetail'
import { SubjectList } from './pages/SubjectList'
import { SubjectNew } from './pages/SubjectNew'
import { SubjectEdit } from './pages/SubjectEdit'
import { Settings } from './pages/Settings'
import { Login } from './pages/Login'
import { Setup } from './pages/Setup'
import { UserAdmin } from './pages/UserAdmin'
import { PriceItemList } from './pages/PriceItemList'
import { PriceItemDetail } from './pages/PriceItemDetail'

function Sidebar() {
  const { user, multiUser, logout } = useAuth()
  const [companyName, setCompanyName] = useState<string>('')

  useEffect(() => {
    api.settings.get().then(s => setCompanyName(s.company_name ?? '')).catch(() => {})
  }, [])

  const navItem = ({ isActive }: { isActive: boolean }) =>
    `flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
      isActive
        ? 'bg-slate-800 text-white'
        : 'text-slate-400 hover:bg-slate-800 hover:text-white'
    }`

  return (
    <aside className="fixed inset-y-0 left-0 w-56 bg-slate-900 flex flex-col">
      {/* Logo */}
      <div className="flex items-center gap-2 px-4 py-5 border-b border-slate-800">
        <div className="h-7 w-7 rounded-md bg-violet-600 flex items-center justify-center">
          <span className="text-white text-xs font-bold">N</span>
        </div>
        <span className="text-white font-semibold text-sm tracking-tight">NanoFaktura</span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 px-3 py-4 space-y-1">
        <NavLink to="/" className={navItem} end>
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
          </svg>
          Přehled
        </NavLink>
        <NavLink to="/invoices" className={navItem}>
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
          Faktury
        </NavLink>
        <NavLink to="/subjects" className={navItem}>
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          Kontakty
        </NavLink>
        <NavLink to="/price-items" className={navItem}>
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
          </svg>
          Ceník
        </NavLink>
        <NavLink to="/settings" className={navItem}>
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 010 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 010-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28z M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          Nastavení
        </NavLink>

        {/* Správa uživatelů — pouze superadmin */}
        {multiUser && user?.role === 'superadmin' && (
          <NavLink to="/admin/users" className={navItem}>
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
            </svg>
            Uživatelé
          </NavLink>
        )}
      </nav>

      {/* Footer */}
      <div className="px-4 py-3 border-t border-slate-800">
        <div className="flex items-center justify-between">
          <div className="min-w-0">
            {companyName && (
              <p className="text-xs text-slate-300 font-medium truncate">{companyName}</p>
            )}
            {multiUser && user ? (
              <p className="text-xs text-slate-500 truncate">{user.username}</p>
            ) : (
              <p className="text-xs text-slate-500">v1.0.0</p>
            )}
          </div>
          {multiUser && user && (
            <button
              onClick={logout}
              title="Odhlásit se"
              className="ml-2 p-1.5 rounded text-slate-500 hover:text-slate-300 hover:bg-slate-800 transition-colors flex-shrink-0"
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
              </svg>
            </button>
          )}
        </div>
      </div>
    </aside>
  )
}

function AppShell() {
  const { user, multiUser, initialized, loading } = useAuth()

  if (loading) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center">
        <svg className="animate-spin h-6 w-6 text-violet-600" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      </div>
    )
  }

  return (
    <Routes>
      {/* Setup wizard — první spuštění */}
      <Route path="/setup" element={<Setup />} />

      {/* Login — pouze multi-user mód */}
      <Route path="/login" element={
        !initialized ? <Navigate to="/setup" replace />
        : multiUser && !user ? <Login />
        : <Navigate to={new URLSearchParams(window.location.search).get('returnTo') || '/'} replace />
      } />

      {/* Všechny ostatní routy */}
      <Route path="*" element={
        !initialized ? <Navigate to="/setup" replace />
        : multiUser && !user
          ? <Navigate to="/login" replace />
          : (
            <div className="min-h-screen bg-slate-50 flex">
              <Sidebar />
              <main className="ml-56 flex-1 min-h-screen">
                <ToastContainer />
                <Routes>
                  <Route path="/"                    element={<InvoiceList />} />
                  <Route path="/invoices"            element={<InvoiceList />} />
                  <Route path="/invoices/new"        element={<InvoiceNew />} />
                  <Route path="/invoices/:id/edit"   element={<InvoiceNew />} />
                  <Route path="/invoices/:id"        element={<InvoiceDetail />} />
                  <Route path="/subjects"            element={<SubjectList />} />
                  <Route path="/subjects/new"        element={<SubjectNew />} />
                  <Route path="/subjects/:id/edit"  element={<SubjectEdit />} />
                  <Route path="/price-items"         element={<PriceItemList />} />
                  <Route path="/price-items/new"     element={<PriceItemDetail />} />
                  <Route path="/price-items/:id"     element={<PriceItemDetail />} />
                  <Route path="/settings"            element={<Settings />} />
                  {multiUser && user?.role === 'superadmin' && (
                    <Route path="/admin/users"       element={<UserAdmin />} />
                  )}
                </Routes>
              </main>
            </div>
          )
      } />
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppShell />
      </AuthProvider>
    </BrowserRouter>
  )
}
