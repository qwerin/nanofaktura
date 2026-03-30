import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type PriceItemInput, type StockMovement } from '../api/client'
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

const EMPTY: PriceItemInput = {
  name: '',
  catalog_no: '',
  ean: '',
  unit_name: 'ks',
  unit_price_hal: 0,
  vat_rate_bps: 0,
  track_stock: false,
  allow_negative_stock: false,
}

export function PriceItemDetail() {
  const { id } = useParams<{ id: string }>()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()

  const [form, setForm] = useState<PriceItemInput>(EMPTY)
  const [stockQty, setStockQty] = useState<number>(0)
  const [movements, setMovements] = useState<StockMovement[]>([])
  const [loading, setLoading] = useState(!isNew)
  const [saving, setSaving] = useState(false)

  // Ruční pohyb
  const [movQty, setMovQty] = useState('')
  const [movNote, setMovNote] = useState('')
  const [movSaving, setMovSaving] = useState(false)

  const loadMovements = async (itemId: number) => {
    const movs = await api.priceItems.movements.list(itemId)
    setMovements(movs)
  }

  useEffect(() => {
    if (isNew) return
    api.priceItems.get(Number(id))
      .then(item => {
        setForm({
          name: item.name,
          catalog_no: item.catalog_no ?? '',
          ean: item.ean ?? '',
          unit_name: item.unit_name,
          unit_price_hal: item.unit_price_hal,
          vat_rate_bps: item.vat_rate_bps,
          track_stock: item.track_stock,
          allow_negative_stock: item.allow_negative_stock,
        })
        setStockQty(item.stock_quantity ?? 0)
        return loadMovements(Number(id))
      })
      .catch((e: Error) => toast(e.message, 'error'))
      .finally(() => setLoading(false))
  }, [id, isNew])

  const set = <K extends keyof typeof form>(field: K) =>
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.type === 'number' ? Number(e.target.value) : e.target.value
      setForm(f => ({ ...f, [field]: val }))
    }

  const payload = (): PriceItemInput => ({
    ...form,
    catalog_no: form.catalog_no?.trim() || undefined,
    ean: form.ean?.trim() || undefined,
  })

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      if (isNew) {
        const item = await api.priceItems.create(payload())
        toast('Položka uložena.')
        navigate(`/price-items/${item.id}`)
      } else {
        const item = await api.priceItems.update(Number(id), payload())
        setStockQty(item.stock_quantity ?? 0)
        toast('Položka uložena.')
      }
    } catch (err) {
      toast((err as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }

  const archive = async () => {
    try {
      await api.priceItems.archive(Number(id))
      toast('Položka archivována.')
      navigate('/price-items')
    } catch (err) {
      toast((err as Error).message, 'error')
    }
  }

  const addMovement = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!movQty) return
    setMovSaving(true)
    try {
      await api.priceItems.movements.create(Number(id), { quantity: movQty, note: movNote })
      setMovQty(''); setMovNote('')
      const item = await api.priceItems.get(Number(id))
      setStockQty(item.stock_quantity ?? 0)
      await loadMovements(Number(id))
      toast('Pohyb přidán.')
    } catch (err) {
      toast((err as Error).message, 'error')
    } finally {
      setMovSaving(false)
    }
  }

  if (loading) return (
    <div className="flex items-center justify-center py-24 text-slate-400">
      <svg className="animate-spin h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
      Načítám…
    </div>
  )

  return (
    <div className="p-8 max-w-3xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">
            {isNew ? 'Nová položka ceníku' : (form.name || 'Detail položky')}
          </h1>
          {!isNew && form.track_stock && (
            <p className="mt-1 text-sm text-slate-500">
              Aktuální stav skladu: <span className={`font-semibold ${stockQty < 0 ? 'text-red-600' : 'text-slate-800'}`}>
                {stockQty} {form.unit_name}
              </span>
            </p>
          )}
        </div>
        {!isNew && (
          <Button variant="outline" onClick={archive} className="text-red-600 hover:text-red-700 border-red-200 hover:border-red-300">
            Archivovat
          </Button>
        )}
      </div>

      <form onSubmit={submit} className="space-y-5">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-semibold text-slate-700">Základní údaje</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Field name="Název" required>
              <Input required value={form.name} onChange={set('name')} />
            </Field>

            <div className="grid grid-cols-2 gap-4">
              <Field name="Katalogové číslo">
                <Input value={form.catalog_no ?? ''} onChange={set('catalog_no')} placeholder="AB-12345" />
              </Field>
              <Field name="EAN">
                <Input value={form.ean ?? ''} onChange={set('ean')} placeholder="8594XXX" maxLength={13} />
              </Field>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <Field name="Jednotka">
                <Input value={form.unit_name} onChange={set('unit_name')} placeholder="ks" />
              </Field>
              <Field name="Cena bez DPH (Kč)">
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  value={form.unit_price_hal / 100}
                  onChange={e => setForm(f => ({ ...f, unit_price_hal: Math.round(parseFloat(e.target.value) * 100) }))}
                />
              </Field>
              <Field name="Sazba DPH">
                <Select
                  value={String(form.vat_rate_bps)}
                  onValueChange={v => setForm(f => ({ ...f, vat_rate_bps: Number(v) }))}
                >
                  <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="0">0 %</SelectItem>
                    <SelectItem value="1200">12 %</SelectItem>
                    <SelectItem value="2100">21 %</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-semibold text-slate-700">Sklad</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border-slate-300 text-violet-600 focus:ring-violet-500"
                checked={form.track_stock}
                onChange={e => setForm(f => ({ ...f, track_stock: e.target.checked }))}
              />
              <span className="text-sm text-slate-700">Sledovat stav skladu</span>
            </label>
            {form.track_stock && (
              <label className="flex items-center gap-3 cursor-pointer ml-7">
                <input
                  type="checkbox"
                  className="h-4 w-4 rounded border-slate-300 text-violet-600 focus:ring-violet-500"
                  checked={form.allow_negative_stock}
                  onChange={e => setForm(f => ({ ...f, allow_negative_stock: e.target.checked }))}
                />
                <span className="text-sm text-slate-700">Povolit záporný stav skladu</span>
              </label>
            )}
          </CardContent>
        </Card>

        <div className="flex justify-between items-center">
          <Button type="button" variant="outline" onClick={() => navigate('/price-items')}>Zpět</Button>
          <Button type="submit" disabled={saving}>{saving ? 'Ukládám…' : 'Uložit'}</Button>
        </div>
      </form>

      {/* ── Skladové pohyby (jen pro existující položky se sledováním skladu) ── */}
      {!isNew && form.track_stock && (
        <div className="mt-8 space-y-4">
          <h2 className="text-base font-semibold text-slate-800">Pohyby skladu</h2>

          {/* Ruční pohyb */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-semibold text-slate-700">Přidat pohyb</CardTitle>
            </CardHeader>
            <CardContent>
              <form onSubmit={addMovement} className="flex gap-3 items-end">
                <div className="w-36">
                  <Label className="block text-xs font-medium text-slate-600 mb-1">
                    Množství <span className="text-slate-400">(+ příjem, − výdej)</span>
                  </Label>
                  <Input
                    type="number"
                    step="0.001"
                    placeholder="např. 10 nebo -5"
                    value={movQty}
                    onChange={e => setMovQty(e.target.value)}
                    required
                  />
                </div>
                <div className="flex-1">
                  <Label className="block text-xs font-medium text-slate-600 mb-1">Poznámka</Label>
                  <Input
                    placeholder="Naskladnění, inventura…"
                    value={movNote}
                    onChange={e => setMovNote(e.target.value)}
                  />
                </div>
                <Button type="submit" disabled={movSaving}>
                  {movSaving ? 'Přidávám…' : 'Přidat'}
                </Button>
              </form>
            </CardContent>
          </Card>

          {/* Historie pohybů */}
          {movements.length > 0 && (
            <div className="rounded-xl border border-slate-200 bg-white overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-slate-50 border-b border-slate-200 text-xs font-semibold text-slate-400 uppercase tracking-wide">
                    <th className="text-left p-4">Datum</th>
                    <th className="text-right p-4">Množství</th>
                    <th className="text-left p-4">Poznámka</th>
                    <th className="text-left p-4">Faktura</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {movements.map(m => {
                    const qty = parseFloat(m.quantity)
                    return (
                      <tr key={m.id}>
                        <td className="p-4 text-slate-500">
                          {m.created_at ? new Date(m.created_at).toLocaleDateString('cs-CZ') : '—'}
                        </td>
                        <td className={`p-4 text-right font-mono font-semibold ${qty < 0 ? 'text-red-600' : 'text-emerald-600'}`}>
                          {qty > 0 ? '+' : ''}{m.quantity} {form.unit_name}
                        </td>
                        <td className="p-4 text-slate-600">{m.note || '—'}</td>
                        <td className="p-4 text-slate-400 text-xs">
                          {m.invoice_id ? `#${m.invoice_id}` : '—'}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
