import { useEffect, useState } from 'react'
import { api, type User, type UserRole } from '../api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

const ROLE_LABELS: Record<UserRole, string> = {
  superadmin: 'Superadmin',
  admin: 'Admin',
  user: 'Uživatel',
}

function RoleBadge({ role }: { role: UserRole }) {
  const colors: Record<UserRole, string> = {
    superadmin: 'bg-violet-100 text-violet-700',
    admin: 'bg-blue-100 text-blue-700',
    user: 'bg-slate-100 text-slate-600',
  }
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${colors[role]}`}>
      {ROLE_LABELS[role]}
    </span>
  )
}

export function UserAdmin() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newEmail, setNewEmail] = useState('')
  const [newRole, setNewRole] = useState<UserRole>('user')
  const [saving, setSaving] = useState(false)
  const [resetId, setResetId] = useState<number | null>(null)
  const [resetPassword, setResetPassword] = useState('')

  const load = () => {
    setLoading(true)
    api.users.list()
      .then(setUsers)
      .catch(e => setError((e as Error).message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const createUser = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await api.users.create({ username: newUsername, password: newPassword, email: newEmail || undefined, role: newRole })
      setNewUsername(''); setNewPassword(''); setNewEmail(''); setNewRole('user')
      setShowCreate(false)
      load()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const toggleActive = async (user: User) => {
    try {
      await api.users.update(user.id!, { is_active: !user.is_active })
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  const doResetPassword = async (id: number) => {
    if (!resetPassword) return
    try {
      await api.users.resetPassword(id, resetPassword)
      setResetId(null); setResetPassword('')
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Správa uživatelů</h1>
          <p className="mt-1 text-sm text-slate-500">Superadmin může přidávat, deaktivovat a resetovat hesla uživatelů.</p>
        </div>
        <Button onClick={() => setShowCreate(v => !v)}>
          {showCreate ? 'Zrušit' : '+ Nový uživatel'}
        </Button>
      </div>

      {showCreate && (
        <Card className="mb-6">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-semibold text-slate-700">Nový uživatel</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={createUser} className="grid grid-cols-2 gap-4">
              <div>
                <Label className="text-xs text-slate-600 mb-1 block">Uživatelské jméno *</Label>
                <Input required value={newUsername} onChange={e => setNewUsername(e.target.value)} />
              </div>
              <div>
                <Label className="text-xs text-slate-600 mb-1 block">Heslo *</Label>
                <Input required type="password" minLength={8} value={newPassword} onChange={e => setNewPassword(e.target.value)} />
              </div>
              <div>
                <Label className="text-xs text-slate-600 mb-1 block">E-mail</Label>
                <Input type="email" value={newEmail} onChange={e => setNewEmail(e.target.value)} />
              </div>
              <div>
                <Label className="text-xs text-slate-600 mb-1 block">Role</Label>
                <Select value={newRole} onValueChange={v => setNewRole(v as UserRole)}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">Uživatel</SelectItem>
                    <SelectItem value="admin">Admin</SelectItem>
                    <SelectItem value="superadmin">Superadmin</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="col-span-2 flex justify-end">
                <Button type="submit" disabled={saving}>{saving ? 'Ukládám…' : 'Vytvořit'}</Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {error && (
        <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-sm text-red-700 mb-4">{error}</div>
      )}

      {loading ? (
        <p className="text-sm text-slate-400">Načítám…</p>
      ) : (
        <Card>
          <CardContent className="p-0">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 text-xs font-semibold text-slate-400 uppercase tracking-wide">
                  <th className="text-left p-4">Uživatel</th>
                  <th className="text-left p-4">E-mail</th>
                  <th className="text-left p-4">Role</th>
                  <th className="text-left p-4">Stav</th>
                  <th className="p-4" />
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {users.map(u => (
                  <tr key={u.id}>
                    <td className="p-4 font-medium text-slate-800">{u.username}</td>
                    <td className="p-4 text-slate-500">{u.email || '—'}</td>
                    <td className="p-4"><RoleBadge role={u.role} /></td>
                    <td className="p-4">
                      <span className={`text-xs font-medium ${u.is_active ? 'text-emerald-600' : 'text-slate-400'}`}>
                        {u.is_active ? 'Aktivní' : 'Deaktivován'}
                      </span>
                    </td>
                    <td className="p-4">
                      <div className="flex items-center gap-2 justify-end">
                        {resetId === u.id ? (
                          <>
                            <Input
                              type="password"
                              placeholder="Nové heslo"
                              value={resetPassword}
                              onChange={e => setResetPassword(e.target.value)}
                              className="w-36 h-7 text-xs"
                            />
                            <Button size="sm" variant="outline" onClick={() => doResetPassword(u.id!)}>
                              Uložit
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => { setResetId(null); setResetPassword('') }}>
                              Zrušit
                            </Button>
                          </>
                        ) : (
                          <>
                            <Button size="sm" variant="outline" onClick={() => setResetId(u.id!)}>
                              Reset hesla
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => toggleActive(u)}>
                              {u.is_active ? 'Deaktivovat' : 'Aktivovat'}
                            </Button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
