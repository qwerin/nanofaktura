import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type Subject, type SubjectInput } from '../api/client'
import { toast } from '../components/Toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
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

function Field({ name, required, children }: { name: string; required?: boolean; children: React.ReactNode }) {
  return (
    <div>
      <Label className="block text-xs font-medium text-slate-600 mb-1">
        {name}{required && <span className="ml-0.5 text-red-500">*</span>}
      </Label>
      {children}
    </div>
  )
}

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-semibold text-slate-700">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">{children}</CardContent>
    </Card>
  )
}

export function SubjectEdit() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [saving, setSaving] = useState(false)
  const [aresLoading, setAresLoading] = useState(false)
  const [subjectName, setSubjectName] = useState('')
  const [form, setForm] = useState<SubjectInput | null>(null)

  useEffect(() => {
    api.subjects.get(Number(id))
      .then((s: Subject) => {
        setSubjectName(s.name)
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        const { id: _id, created_at: _ca, updated_at: _ua, deleted_at: _da, user_id: _uid, $schema: _s, ...input } = s
        setForm(input)
      })
      .catch((e: Error) => {
        toast(e.message, 'error')
        navigate('/subjects')
      })
  }, [id])

  const set = <K extends keyof SubjectInput>(field: K) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const val = e.target.type === 'number' ? Number(e.target.value) : e.target.value
      setForm(f => f ? { ...f, [field]: val } : f)
    }

  const fetchAres = async () => {
    const ic = form?.registration_no ?? ''
    if (ic.length !== 8) return
    setAresLoading(true)
    try {
      const s = await api.ares.lookup(ic)
      setForm(f => f ? {
        ...f,
        name: s.name || f.name,
        vat_no: s.dic,
        street: s.street,
        city: s.city,
        zip: s.zip,
      } : f)
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setAresLoading(false)
    }
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!form) return
    setSaving(true)
    try {
      await api.subjects.update(Number(id), form)
      navigate('/subjects')
    } catch (err) {
      toast((err as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }

  if (!form) {
    return (
      <div className="flex items-center justify-center py-16 text-slate-400">
        <svg className="animate-spin h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        Načítám…
      </div>
    )
  }

  return (
    <div className="p-8 max-w-3xl">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold text-slate-900">Upravit kontakt</h1>
        <p className="mt-1 text-sm text-slate-500">{subjectName}</p>
      </div>

      <form onSubmit={submit} className="space-y-5">

        <SectionCard title="Základní údaje">
          <div className="grid grid-cols-2 gap-4">
            <Field name="Typ kontaktu">
              <Select
                value={form.type ?? 'customer'}
                onValueChange={(val) => setForm(f => f ? { ...f, type: val as SubjectInput['type'] } : f)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="customer">Zákazník</SelectItem>
                  <SelectItem value="supplier">Dodavatel</SelectItem>
                  <SelectItem value="both">Obojí</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>

          <Field name="IČO">
            <div className="flex gap-2">
              <Input value={form.registration_no ?? ''} onChange={set('registration_no')}
                maxLength={8} placeholder="12345678" className="flex-1" />
              <Button type="button" variant="outline" disabled={aresLoading} onClick={fetchAres}>
                {aresLoading ? '…' : 'ARES'}
              </Button>
            </div>
          </Field>

          <Field name="Název" required>
            <Input required value={form.name} onChange={set('name')} />
          </Field>

          <Field name="DIČ">
            <Input value={form.vat_no ?? ''} onChange={set('vat_no')} />
          </Field>

          <Field name="Ulice">
            <Input value={form.street ?? ''} onChange={set('street')} />
          </Field>

          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">
              <Field name="Město">
                <Input value={form.city ?? ''} onChange={set('city')} />
              </Field>
            </div>
            <Field name="PSČ">
              <Input value={form.zip ?? ''} onChange={set('zip')} maxLength={5} />
            </Field>
          </div>
        </SectionCard>

        <SectionCard title="Kontakt">
          <div className="grid grid-cols-2 gap-4">
            <Field name="E-mail">
              <Input type="email" value={form.email ?? ''} onChange={set('email')} />
            </Field>
            <Field name="Telefon">
              <Input type="tel" value={form.phone ?? ''} onChange={set('phone')} />
            </Field>
          </div>
          <Field name="Web">
            <Input type="url" value={form.website ?? ''} onChange={set('website')}
              placeholder="https://" />
          </Field>
        </SectionCard>

        <SectionCard title="Platební informace">
          <div className="grid grid-cols-2 gap-4">
            <Field name="Bankovní účet">
              <Input value={form.bank_account ?? ''} onChange={set('bank_account')}
                placeholder="123456789/0800" />
            </Field>
            <Field name="IBAN">
              <Input value={form.iban ?? ''} onChange={set('iban')} />
            </Field>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <Field name="Výchozí způsob platby">
              <Select
                value={form.default_payment_method ?? 'bank'}
                onValueChange={(val) => setForm(f => f ? { ...f, default_payment_method: val as SubjectInput['default_payment_method'] } : f)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bank">Bankovní převod</SelectItem>
                  <SelectItem value="cash">Hotovost</SelectItem>
                  <SelectItem value="card">Karta</SelectItem>
                  <SelectItem value="cod">Dobírka</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field name="Výchozí splatnost (dní)">
              <Input type="number" min={0} value={form.default_due ?? 14} onChange={set('default_due')} />
            </Field>
          </div>
          <Field name="Interní poznámka">
            <Textarea value={form.note ?? ''} onChange={set('note')}
              rows={3} className="resize-none" />
          </Field>
        </SectionCard>

        <div className="flex justify-end gap-3 pb-8">
          <Button type="button" variant="outline" onClick={() => navigate('/subjects')}>
            Zrušit
          </Button>
          <Button type="submit" disabled={saving}>
            {saving ? 'Ukládám…' : 'Uložit změny'}
          </Button>
        </div>
      </form>
    </div>
  )
}
