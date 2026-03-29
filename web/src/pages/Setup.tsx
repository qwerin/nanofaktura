import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

function Field({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 mb-1">
        {label}{required && <span className="text-red-500 ml-0.5">*</span>}
      </label>
      {children}
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h2 className="text-sm font-semibold text-slate-700 mb-3 pb-2 border-b border-slate-100">{title}</h2>
      <div className="space-y-3">{children}</div>
    </div>
  )
}

export function Setup() {
  const { refresh } = useAuth()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [multiUser, setMultiUser] = useState(false)

  // Firemní údaje
  const [companyName, setCompanyName] = useState('')
  const [registrationNo, setRegistrationNo] = useState('')
  const [vatNo, setVatNo] = useState('')
  const [vatExempt, setVatExempt] = useState(true)
  const [street, setStreet] = useState('')
  const [city, setCity] = useState('')
  const [zip, setZip] = useState('')
  const [bankAccount, setBankAccount] = useState('')
  const [iban, setIban] = useState('')

  // Admin účet (jen pro multi-user)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [password2, setPassword2] = useState('')

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (multiUser && password !== password2) {
      setError('Hesla se neshodují')
      return
    }
    setLoading(true); setError(null)
    try {
      const res = await fetch('/api/setup/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          company_name: companyName,
          company_street: street,
          company_city: city,
          company_zip: zip,
          registration_no: registrationNo,
          vat_no: vatNo,
          vat_exempt: vatExempt,
          bank_account: bankAccount,
          iban,
          multi_user: multiUser,
          username: multiUser ? username : undefined,
          email: multiUser ? email : undefined,
          password: multiUser ? password : undefined,
        }),
      })
      const json = await res.json()
      if (!res.ok) throw new Error(json.title ?? 'Setup selhal')
      await refresh()
      navigate('/')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-slate-50 flex items-center justify-center py-12">
      <div className="w-full max-w-lg">
        {/* Logo */}
        <div className="flex items-center justify-center gap-2 mb-8">
          <div className="h-9 w-9 rounded-lg bg-violet-600 flex items-center justify-center">
            <span className="text-white text-sm font-bold">N</span>
          </div>
          <span className="text-slate-900 font-semibold text-xl tracking-tight">NanoFaktura</span>
        </div>

        <div className="bg-white rounded-xl shadow-sm border border-slate-200 p-8">
          <h1 className="text-lg font-semibold text-slate-900 mb-1">Vítejte v NanoFaktura</h1>
          <p className="text-sm text-slate-500 mb-6">Vyplňte základní nastavení pro spuštění aplikace.</p>

          <form onSubmit={submit} className="space-y-6">

            {/* Vaše firma */}
            <Section title="Vaše firma">
              <Field label="Název firmy / jméno" required>
                <Input required value={companyName} onChange={e => setCompanyName(e.target.value)} placeholder="Jan Novák" />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="IČO">
                  <Input value={registrationNo} onChange={e => setRegistrationNo(e.target.value)} maxLength={8} placeholder="12345678" />
                </Field>
                <Field label="DIČ">
                  <Input value={vatNo} onChange={e => setVatNo(e.target.value)} placeholder="CZ12345678" />
                </Field>
              </div>
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={vatExempt}
                  onChange={e => setVatExempt(e.target.checked)}
                  className="h-4 w-4 rounded border-slate-300 text-violet-600 focus:ring-violet-500"
                />
                <span className="text-sm text-slate-700">Nejsem plátce DPH</span>
              </label>
              <Field label="Ulice a číslo">
                <Input value={street} onChange={e => setStreet(e.target.value)} placeholder="Hlavní 1" />
              </Field>
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <Field label="Město">
                    <Input value={city} onChange={e => setCity(e.target.value)} placeholder="Praha" />
                  </Field>
                </div>
                <Field label="PSČ">
                  <Input value={zip} onChange={e => setZip(e.target.value)} maxLength={5} placeholder="11000" />
                </Field>
              </div>
              <Field label="Číslo účtu">
                <Input value={bankAccount} onChange={e => setBankAccount(e.target.value)} placeholder="123456789/0800" />
              </Field>
              <Field label="IBAN">
                <Input value={iban} onChange={e => setIban(e.target.value)} placeholder="CZ6508000000192000145399" />
              </Field>
            </Section>

            {/* Režim aplikace */}
            <Section title="Režim aplikace">
              <label className="flex items-start gap-3 cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={multiUser}
                  onChange={e => setMultiUser(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded border-slate-300 text-violet-600 focus:ring-violet-500"
                />
                <div>
                  <span className="text-sm font-medium text-slate-800">Více uživatelů</span>
                  <p className="text-xs text-slate-500 mt-0.5">
                    Každý uživatel se přihlašuje svým heslem a vidí pouze svá data.
                    Bez zaškrtnutí poběží aplikace bez přihlašování.
                  </p>
                </div>
              </label>
            </Section>

            {/* Admin účet — zobrazí se pouze pro multi-user */}
            {multiUser && (
              <Section title="Administrátorský účet">
                <p className="text-xs text-slate-500 -mt-1">Tento účet bude mít plný přístup k nastavení a správě uživatelů.</p>
                <Field label="Uživatelské jméno" required>
                  <Input required value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" />
                </Field>
                <Field label="E-mail">
                  <Input type="email" value={email} onChange={e => setEmail(e.target.value)} autoComplete="email" />
                </Field>
                <Field label="Heslo" required>
                  <Input type="password" required minLength={8} value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" />
                </Field>
                <Field label="Heslo znovu" required>
                  <Input type="password" required value={password2} onChange={e => setPassword2(e.target.value)} autoComplete="new-password" />
                </Field>
              </Section>
            )}

            {error && (
              <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-sm text-red-700">{error}</div>
            )}

            <Button type="submit" disabled={loading} className="w-full">
              {loading ? 'Ukládám…' : 'Spustit aplikaci'}
            </Button>
          </form>
        </div>
      </div>
    </div>
  )
}
