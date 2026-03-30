import { createContext, useContext, useEffect, useState } from 'react'
import { api, type User } from '../api/client'

interface AuthState {
  user: User | null
  multiUser: boolean
  initialized: boolean
  loading: boolean
  login:   (username: string, password: string) => Promise<void>
  logout:  () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthState>({
  user: null,
  multiUser: false,
  initialized: true,
  loading: true,
  login:   async () => {},
  logout:  async () => {},
  refresh: async () => {},
})

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [multiUser, setMultiUser] = useState(false)
  const [initialized, setInitialized] = useState(true)
  const [loading, setLoading] = useState(true)

  const refresh = async () => {
    try {
      const health = await api.health()
      setMultiUser(health.multi_user)
      setInitialized(health.initialized)
      if (health.multi_user && health.initialized) {
        // Přímý fetch — záměrně obcházíme api.request(), který by 401 přeměnil
        // na window.location.href redirect. Zde 401 jen znamená "nepřihlášen".
        const meRes = await fetch('/api/auth/me')
        if (meRes.ok) {
          const json = await meRes.json()
          const { $schema: _s, ...me } = json as Record<string, unknown>
          setUser(me as User)
        } else {
          setUser(null)
        }
      }
    } catch {
      // Backend nedostupný — zachováme stav
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { refresh() }, [])

  const login = async (username: string, password: string) => {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })
    const json = await res.json()
    if (!res.ok) throw new Error(json.title ?? 'Přihlášení selhalo')
    setUser(json as User)
  }

  const logout = async () => {
    await fetch('/api/auth/logout', { method: 'POST' })
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, multiUser, initialized, loading, login, logout, refresh }}>
      {children}
    </AuthContext.Provider>
  )
}

export const useAuth = () => useContext(AuthContext)
