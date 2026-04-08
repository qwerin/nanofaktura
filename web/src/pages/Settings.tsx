import { useEffect, useState } from 'react'
import { api, type Settings as SettingsType, type SettingsInput, type NumberFormat, type NumberFormatInput } from '../api/client'
import { czIban, czSwift, validateCzAccount } from '../utils/iban'
import { toast } from '../components/Toast'
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

function Field({ name, children }: { name: string; children: React.ReactNode }) {
  return (
    <div>
      <Label className="block text-xs font-medium text-slate-600 mb-1">{name}</Label>
      {children}
    </div>
  )
}

export function Settings() {
  const [settings, setSettings] = useState<SettingsInput>({})
  const [formats, setFormats] = useState<NumberFormat[]>([])
  const [saving, setSaving] = useState(false)
  const [savingSection, setSavingSection] = useState<string | null>(null)

  // New format form state
  const [showNewFormat, setShowNewFormat] = useState(false)
  const [newFormat, setNewFormat] = useState<NumberFormatInput>({
    document_type: 'invoice',
    label: '',
    pattern: '{YYYY}{NNN}',
    next_number: 1,
    padding_width: 3,
  })

  // Edit format state
  const [editingFormatId, setEditingFormatId] = useState<number | null>(null)
  const [editFormat, setEditFormat] = useState<NumberFormatInput | null>(null)

  // Preview number state
  const [previewNumbers, setPreviewNumbers] = useState<Record<number, string>>({})

  useEffect(() => {
    loadSettings()
  }, [])

  async function loadSettings() {
    try {
      const s = await api.settings.get()
      setSettings(s)
      setFormats(s.number_formats ?? [])
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  async function saveCompany(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true); setSavingSection('company')
    try {
      const updated = await api.settings.update({
        company_name: settings.company_name,
        company_street: settings.company_street,
        company_city: settings.company_city,
        company_zip: settings.company_zip,
        company_country: settings.company_country,
        registration_no: settings.registration_no,
        vat_no: settings.vat_no,
        vat_exempt: settings.vat_exempt,
      })
      setSettings(updated)
      toast('Údaje firmy uloženy.')
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSaving(false); setSavingSection(null)
    }
  }

  async function savePayment(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true); setSavingSection('payment')
    try {
      const updated = await api.settings.update({
        bank_account: settings.bank_account,
        iban: settings.iban,
        swift_bic: settings.swift_bic,
        default_payment_method: settings.default_payment_method,
        default_due: settings.default_due,
        default_currency: settings.default_currency,
      })
      setSettings(updated)
      toast('Platební údaje uloženy.')
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSaving(false); setSavingSection(null)
    }
  }

  async function createFormat(e: React.FormEvent) {
    e.preventDefault()
    try {
      const created = await api.settings.formats.create(newFormat)
      setFormats(f => [...f, created])
      setShowNewFormat(false)
      setNewFormat({ document_type: 'invoice', label: '', pattern: '{YYYY}{NNN}', next_number: 1, padding_width: 3 })
      toast('Číselná řada přidána.')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  async function saveFormat(e: React.FormEvent) {
    e.preventDefault()
    if (!editFormat || editingFormatId == null) return
    try {
      const updated = await api.settings.formats.update(editingFormatId, editFormat)
      setFormats(fs => fs.map(f => f.id === editingFormatId ? updated : f))
      setEditingFormatId(null); setEditFormat(null)
      toast('Číselná řada uložena.')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  async function deleteFormat(id: number) {
    if (!confirm('Opravdu smazat číselnou řadu?')) return
    try {
      await api.settings.formats.delete(id)
      setFormats(fs => fs.filter(f => f.id !== id))
      toast('Číselná řada smazána.')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  async function generateNext(id: number) {
    try {
      const res = await api.settings.formats.next(id)
      setPreviewNumbers(prev => ({ ...prev, [id]: res.number }))
      // Refresh formats to reflect incremented next_number
      const updated = await api.settings.formats.list()
      setFormats(updated)
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const set = <K extends keyof SettingsType>(field: K) =>
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.type === 'checkbox'
        ? (e.target as HTMLInputElement).checked
        : e.target.type === 'number'
          ? Number(e.target.value)
          : e.target.value
      setSettings(s => ({ ...s, [field]: val }))
    }

  const setNF = <K extends keyof NumberFormatInput>(obj: NumberFormatInput, setObj: (nf: NumberFormatInput) => void, field: K) =>
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.type === 'number' ? Number(e.target.value) : e.target.value
      setObj({ ...obj, [field]: val })
    }

  const docTypeLabel = (dt: string) => {
    switch (dt) {
      case 'invoice': return 'Faktura'
      case 'proforma': return 'Proforma'
      case 'correction': return 'Opravný doklad'
      default: return dt
    }
  }

  return (
    <div className="p-4 md:p-8 max-w-3xl space-y-6">
      <div className="mb-2">
        <h1 className="text-2xl font-semibold text-slate-900">Nastavení</h1>
        <p className="mt-1 text-sm text-slate-500">Konfigurace vašeho účtu a výchozích hodnot.</p>
      </div>

      {/* Section 1: Vaše firma */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-semibold text-slate-700">Vaše firma</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={saveCompany} className="space-y-4">
            <Field name="Název firmy">
              <Input value={settings.company_name ?? ''} onChange={set('company_name')} placeholder="Moje firma s.r.o." />
            </Field>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Field name="IČO">
                <Input value={settings.registration_no ?? ''} onChange={set('registration_no')} maxLength={8} placeholder="12345678" />
              </Field>
              <Field name="DIČ">
                <Input value={settings.vat_no ?? ''} onChange={set('vat_no')} placeholder="CZ12345678" />
              </Field>
            </div>
            <Field name="Ulice">
              <Input value={settings.company_street ?? ''} onChange={set('company_street')} placeholder="Václavské nám. 1" />
            </Field>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Field name="Město">
                <Input value={settings.company_city ?? ''} onChange={set('company_city')} placeholder="Praha" />
              </Field>
              <Field name="PSČ">
                <Input value={settings.company_zip ?? ''} onChange={set('company_zip')} maxLength={5} placeholder="11000" />
              </Field>
            </div>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="vat_exempt"
                checked={settings.vat_exempt ?? true}
                onChange={set('vat_exempt')}
                className="h-4 w-4 rounded border-slate-300 text-violet-600 focus:ring-violet-500"
              />
              <Label htmlFor="vat_exempt" className="text-sm text-slate-700 cursor-pointer">
                Nejsem plátce DPH (osvobozeno)
              </Label>
            </div>
            <div className="flex justify-end pt-1">
              <Button type="submit" disabled={saving && savingSection === 'company'}>
                {saving && savingSection === 'company' ? 'Ukládám…' : 'Uložit'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Section 2: Platební údaje */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-semibold text-slate-700">Platební údaje</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={savePayment} className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Field name="Bankovní účet">
                <Input
                  value={settings.bank_account ?? ''}
                  onChange={e => {
                    const val = e.target.value
                    setSettings(s => ({ ...s, bank_account: val, iban: czIban(val) || s.iban || '', swift_bic: czSwift(val) || s.swift_bic || '' }))
                  }}
                  placeholder="123456789/0800"
                />
                {settings.bank_account && !validateCzAccount(settings.bank_account) && (
                  <p className="text-xs text-amber-600 mt-1">Číslo účtu nevyhovuje kontrole mod-11</p>
                )}
              </Field>
              <Field name="IBAN">
                <Input value={settings.iban ?? ''} onChange={set('iban')} placeholder="CZ65 0800 0000 0012 3456 7890" />
              </Field>
            </div>
            <Field name="SWIFT/BIC">
              <Input value={settings.swift_bic ?? ''} onChange={set('swift_bic')} placeholder="GIBACZPX" />
            </Field>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <Field name="Způsob platby">
                <Select
                  value={settings.default_payment_method ?? 'bank'}
                  onValueChange={val => setSettings(s => ({ ...s, default_payment_method: val }))}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="bank">Bankovní převod</SelectItem>
                    <SelectItem value="cash">Hotovost</SelectItem>
                    <SelectItem value="card">Karta</SelectItem>
                    <SelectItem value="cod">Dobírka</SelectItem>
                    <SelectItem value="paypal">PayPal</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field name="Splatnost (dní)">
                <Input type="number" min={0} value={settings.default_due ?? 14} onChange={set('default_due')} />
              </Field>
              <Field name="Měna">
                <Input value={settings.default_currency ?? 'CZK'} onChange={set('default_currency')} maxLength={3} placeholder="CZK" />
              </Field>
            </div>
            <div className="flex justify-end pt-1">
              <Button type="submit" disabled={saving && savingSection === 'payment'}>
                {saving && savingSection === 'payment' ? 'Ukládám…' : 'Uložit'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Section 3: PDF šablona */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-semibold text-slate-700">PDF šablona faktury</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {([
              { key: 'classic', label: 'Klasická', desc: 'Tmavé záhlaví, ohraničená tabulka, tradiční styl' },
              { key: 'modern',  label: 'Moderní',  desc: 'Fialový pruh, vzdušný layout, QR kód výrazně' },
              { key: 'minimal', label: 'Minimální', desc: 'Čistě bílá, jen tenké linky, maximální vzduch' },
            ] as const).map(tmpl => (
              <button
                key={tmpl.key}
                type="button"
                onClick={async () => {
                  const updated = await api.settings.update({ invoice_template: tmpl.key }).catch(() => null)
                  if (updated) { setSettings(updated); toast('Šablona uložena.') }
                }}
                className={`rounded-lg border-2 p-4 text-left transition-all ${
                  (settings.invoice_template ?? 'classic') === tmpl.key
                    ? 'border-violet-500 bg-violet-50'
                    : 'border-slate-200 hover:border-slate-300 bg-white'
                }`}
              >
                {/* Preview thumbnail */}
                <div className={`w-full h-16 rounded mb-3 flex flex-col gap-1 p-2 ${
                  tmpl.key === 'classic' ? 'bg-slate-800' :
                  tmpl.key === 'modern'  ? 'bg-white border border-slate-200' :
                  'bg-white border border-slate-100'
                }`}>
                  {tmpl.key === 'classic' && (
                    <>
                      <div className="h-2 w-16 rounded bg-white/80" />
                      <div className="h-1 w-10 rounded bg-white/40" />
                      <div className="mt-1 flex gap-1">
                        <div className="h-1 flex-1 rounded bg-violet-400/60" />
                        <div className="h-1 flex-1 rounded bg-violet-400/40" />
                        <div className="h-1 flex-1 rounded bg-violet-400/40" />
                      </div>
                    </>
                  )}
                  {tmpl.key === 'modern' && (
                    <div className="flex h-full gap-1">
                      <div className="w-1 rounded bg-violet-500" />
                      <div className="flex-1 flex flex-col gap-1 pt-1">
                        <div className="h-2 w-12 rounded bg-violet-500/70" />
                        <div className="h-1 w-8 rounded bg-slate-300" />
                        <div className="mt-1 h-1 w-full rounded bg-slate-200" />
                        <div className="h-1 w-3/4 rounded bg-slate-200" />
                      </div>
                    </div>
                  )}
                  {tmpl.key === 'minimal' && (
                    <div className="flex flex-col gap-1.5 pt-1">
                      <div className="h-2 w-10 rounded bg-slate-800/60" />
                      <div className="h-px w-full bg-slate-300" />
                      <div className="h-1 w-3/4 rounded bg-slate-300" />
                      <div className="h-1 w-1/2 rounded bg-slate-200" />
                    </div>
                  )}
                </div>
                <p className="text-sm font-medium text-slate-800">{tmpl.label}</p>
                <p className="text-xs text-slate-500 mt-0.5">{tmpl.desc}</p>
                {(settings.invoice_template ?? 'classic') === tmpl.key && (
                  <span className="inline-block mt-2 text-xs font-medium text-violet-600">✓ Aktivní</span>
                )}
              </button>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Section 4: Číselné řady */}
      <Card>
        <CardHeader className="pb-3 flex flex-row items-center justify-between">
          <CardTitle className="text-sm font-semibold text-slate-700">Číselné řady dokladů</CardTitle>
          <Button type="button" variant="outline" size="sm" onClick={() => setShowNewFormat(v => !v)}>
            {showNewFormat ? 'Zrušit' : '+ Přidat řadu'}
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* New format inline form */}
          {showNewFormat && (
            <form onSubmit={createFormat} className="border border-slate-200 rounded-lg p-4 space-y-3 bg-slate-50">
              <p className="text-xs font-semibold text-slate-600 uppercase tracking-wide">Nová číselná řada</p>
              <div className="grid grid-cols-2 gap-3">
                <Field name="Typ dokladu">
                  <Select
                    value={newFormat.document_type}
                    onValueChange={val => setNewFormat(f => ({ ...f, document_type: val }))}
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="invoice">Faktura</SelectItem>
                      <SelectItem value="proforma">Proforma</SelectItem>
                      <SelectItem value="correction">Opravný doklad</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                <Field name="Popis">
                  <Input value={newFormat.label ?? ''} onChange={setNF(newFormat, setNewFormat, 'label')} placeholder="Faktury 2025" />
                </Field>
              </div>
              <div className="grid grid-cols-3 gap-3">
                <Field name="Vzor">
                  <Input value={newFormat.pattern} onChange={setNF(newFormat, setNewFormat, 'pattern')} placeholder="{YYYY}{NNN}" />
                </Field>
                <Field name="Další číslo">
                  <Input type="number" min={1} value={newFormat.next_number} onChange={setNF(newFormat, setNewFormat, 'next_number')} />
                </Field>
                <Field name="Šířka">
                  <Input type="number" min={1} max={10} value={newFormat.padding_width ?? 3} onChange={setNF(newFormat, setNewFormat, 'padding_width')} />
                </Field>
              </div>
              <div className="flex justify-end gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => setShowNewFormat(false)}>Zrušit</Button>
                <Button type="submit" size="sm">Přidat</Button>
              </div>
            </form>
          )}

          {/* Formats table */}
          {formats.length === 0 ? (
            <p className="text-sm text-slate-400 text-center py-4">Žádné číselné řady.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-xs text-slate-500 text-left border-b border-slate-200">
                    <th className="pb-2 pr-3 font-medium">Typ dokladu</th>
                    <th className="pb-2 pr-3 font-medium">Popis</th>
                    <th className="pb-2 pr-3 font-medium">Vzor</th>
                    <th className="pb-2 pr-3 font-medium text-right">Další číslo</th>
                    <th className="pb-2 pr-3 font-medium text-right">Šířka</th>
                    <th className="pb-2 font-medium" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {formats.map(fmt => (
                    <tr key={fmt.id}>
                      {editingFormatId === fmt.id && editFormat ? (
                        <>
                          <td className="py-2 pr-2">
                            <Select
                              value={editFormat.document_type}
                              onValueChange={val => setEditFormat(f => f ? { ...f, document_type: val } : f)}
                            >
                              <SelectTrigger className="w-28 h-8 text-xs">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="invoice">Faktura</SelectItem>
                                <SelectItem value="proforma">Proforma</SelectItem>
                                <SelectItem value="correction">Opravný doklad</SelectItem>
                              </SelectContent>
                            </Select>
                          </td>
                          <td className="py-2 pr-2">
                            <Input className="h-8 text-xs" value={editFormat.label ?? ''} onChange={setNF(editFormat, f => setEditFormat(f), 'label')} />
                          </td>
                          <td className="py-2 pr-2">
                            <Input className="h-8 text-xs" value={editFormat.pattern} onChange={setNF(editFormat, f => setEditFormat(f), 'pattern')} />
                          </td>
                          <td className="py-2 pr-2">
                            <Input className="h-8 text-xs w-20 text-right" type="number" min={1} value={editFormat.next_number} onChange={setNF(editFormat, f => setEditFormat(f), 'next_number')} />
                          </td>
                          <td className="py-2 pr-2">
                            <Input className="h-8 text-xs w-16 text-right" type="number" min={1} max={10} value={editFormat.padding_width ?? 3} onChange={setNF(editFormat, f => setEditFormat(f), 'padding_width')} />
                          </td>
                          <td className="py-2">
                            <form onSubmit={saveFormat} className="flex gap-1">
                              <Button type="submit" size="sm" className="h-7 px-2 text-xs">Uložit</Button>
                              <Button type="button" variant="outline" size="sm" className="h-7 px-2 text-xs" onClick={() => { setEditingFormatId(null); setEditFormat(null) }}>×</Button>
                            </form>
                          </td>
                        </>
                      ) : (
                        <>
                          <td className="py-2 pr-3 text-slate-700">{docTypeLabel(fmt.document_type)}</td>
                          <td className="py-2 pr-3 text-slate-500">{fmt.label}</td>
                          <td className="py-2 pr-3 font-mono text-slate-700">{fmt.pattern}</td>
                          <td className="py-2 pr-3 text-right text-slate-700">{fmt.next_number}</td>
                          <td className="py-2 pr-3 text-right text-slate-500">{fmt.padding_width}</td>
                          <td className="py-2">
                            <div className="flex gap-1 justify-end items-center">
                              {previewNumbers[fmt.id!] && (
                                <span className="text-xs font-mono text-violet-700 mr-1">{previewNumbers[fmt.id!]}</span>
                              )}
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                className="h-7 px-2 text-xs"
                                onClick={() => generateNext(fmt.id!)}
                              >
                                Vygenerovat
                              </Button>
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                className="h-7 px-2 text-xs"
                                onClick={() => {
                                  setEditingFormatId(fmt.id!)
                                  // eslint-disable-next-line @typescript-eslint/no-unused-vars
                                  const { id: _id, created_at: _ca, updated_at: _ua, deleted_at: _da, settings_id: _si, $schema: _s, ...input } = fmt
                                  setEditFormat(input)
                                }}
                              >
                                Upravit
                              </Button>
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                className="h-7 px-2 text-xs text-red-500 hover:text-red-700"
                                onClick={() => deleteFormat(fmt.id!)}
                              >
                                Smazat
                              </Button>
                            </div>
                          </td>
                        </>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="text-xs text-slate-400">
            Vzorové tokeny: <code className="font-mono">{'{YYYY}'}</code> rok, <code className="font-mono">{'{YY}'}</code> 2-místný rok, <code className="font-mono">{'{NNN}'}</code> pořadové číslo
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
