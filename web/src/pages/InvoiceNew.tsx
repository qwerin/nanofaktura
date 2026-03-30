import { useEffect, useRef, useState } from 'react'
import { useNavigate, useLocation, useParams } from 'react-router-dom'
import { api, type Invoice, type InvoiceInput, type LineInput, type Subject, type Settings, type NumberFormat, type PriceItem, type PaymentMethod } from '../api/client'
import { toast } from '../components/Toast'
import { formatKc } from '../utils/money'
import { czIban, czSwift } from '../utils/iban'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DateInput } from '../components/DateInput'

const today = () => new Date().toISOString().slice(0, 10)


const DUE_OPTIONS = [0, 7, 14, 21, 30, 45, 60, 90]

const PAYMENT_METHODS: { key: PaymentMethod; label: string }[] = [
  { key: 'bank',   label: 'Banka' },
  { key: 'card',   label: 'Kartou' },
  { key: 'cash',   label: 'Hotově' },
  { key: 'cod',    label: 'Dobírka' },
  { key: 'paypal', label: 'Jiná' },
]

const EMPTY_LINE: LineInput = {
  position: 1,
  name: '',
  quantity: '1',
  unit_name: 'hod',
  unit_price_hal: 0,
  vat_rate_bps: 0,
}

function previewNumber(fmt: NumberFormat): string {
  const y = new Date().getFullYear()
  return fmt.pattern
    .replace('{YYYY}', String(y))
    .replace('{YY}', String(y % 100).padStart(2, '0'))
    .replace('{NNN}', String(fmt.next_number).padStart(fmt.padding_width ?? 3, '0'))
}

function dueDate(issuedOn: string, dueDays: number): string {
  const d = new Date(issuedOn)
  d.setDate(d.getDate() + dueDays)
  return d.toLocaleDateString('cs-CZ', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

// Horizontal form row: right-aligned label + right-side content
function Row({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-0 min-h-[38px]">
      <label className="w-44 shrink-0 pr-4 pt-2 text-sm text-slate-500 text-right leading-tight">
        {label}{required && <span className="text-red-500 ml-0.5">*</span>}
      </label>
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  )
}

export function InvoiceNew() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id: editIdStr } = useParams<{ id?: string }>()
  const editId = editIdStr ? Number(editIdStr) : null
  const isEdit = editId !== null
  const prefill = (location.state as { prefill?: Invoice } | null)?.prefill ?? null

  const [saving, setSaving] = useState(false)
  const [aresLoading, setAresLoading] = useState(false)
  const [priceItems, setPriceItems] = useState<PriceItem[]>([])
  const [showPriceDialog, setShowPriceDialog] = useState(false)
  const [priceQuery, setPriceQuery] = useState('')
  const [subjects, setSubjects] = useState<Subject[]>([])
  const [selectedSubjectId, setSelectedSubjectId] = useState<string>(
    prefill?.subject_id ? String(prefill.subject_id) : ''
  )
  const [invoiceFormat, setInvoiceFormat] = useState<NumberFormat | null>(null)
  const [showMore, setShowMore] = useState(false)
  const [docType, setDocType] = useState<'invoice' | 'proforma'>('invoice')
  const formRef = useRef<HTMLFormElement>(null)

  const [form, setForm] = useState<InvoiceInput>({
    number: '',
    issued_on: today(),
    taxable_fulfillment_due: today(),
    due: prefill?.due ?? 14,
    your_name: '',
    your_registration_no: '',
    your_street: '',
    your_city: '',
    your_zip: '',
    your_country: 'CZ',
    // Client info from prefill
    client_name: prefill?.client_name ?? '',
    client_registration_no: prefill?.client_registration_no ?? '',
    client_vat_no: prefill?.client_vat_no ?? '',
    client_street: prefill?.client_street ?? '',
    client_city: prefill?.client_city ?? '',
    client_zip: prefill?.client_zip ?? '',
    client_country: prefill?.client_country ?? 'CZ',
    subject_id: prefill?.subject_id,
    bank_account: '',
    variable_symbol: '',
    payment_method: prefill?.payment_method ?? 'bank',
    currency: prefill?.currency ?? 'CZK',
    vat_exempt: true,
    note: prefill?.note ?? '',
    // Lines from prefill (strip server-computed ids)
    lines: prefill?.lines?.map((l, i) => ({
      position: l.position ?? i + 1,
      name: l.name,
      quantity: l.quantity,
      unit_name: l.unit_name,
      unit_price_hal: l.unit_price_hal,
      vat_rate_bps: l.vat_rate_bps,
    })) ?? [{ ...EMPTY_LINE }],
  })

  useEffect(() => {
    api.subjects.list().then(setSubjects).catch(() => {})
    api.priceItems.list().then(setPriceItems).catch(() => {})

    if (isEdit && editId) {
      // Edit mode: load existing invoice, skip settings defaults
      api.invoices.get(editId).then(inv => {
        setDocType((inv.document_type as 'invoice' | 'proforma') ?? 'invoice')
        setSelectedSubjectId(inv.subject_id ? String(inv.subject_id) : '')
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        const { id: _id, created_at: _ca, updated_at: _ua, deleted_at: _da, status: _st,
                subtotal: _sub, total_vat_hal: _tv, total: _tot, user_id: _uid, $schema: _s, ...invInput } = inv
        setForm({
          ...invInput,
          lines: inv.lines.map((l, i) => ({
            position: l.position ?? i + 1,
            name: l.name,
            quantity: l.quantity,
            unit_name: l.unit_name,
            unit_price_hal: l.unit_price_hal,
            vat_rate_bps: l.vat_rate_bps,
          })),
        })
        if (inv.note || inv.variable_symbol || inv.bank_account || inv.iban || inv.swift_bic) {
          setShowMore(true)
        }
      }).catch(() => {})
      return
    }

    api.settings.get().then((s: Settings) => {
      const fmt = (s.number_formats ?? []).find(f => f.document_type === 'invoice') ?? null
      setInvoiceFormat(fmt)
      setForm(f => ({
        ...f,
        number: fmt ? previewNumber(fmt) : f.number,
        your_name: s.company_name ?? f.your_name,
        your_street: s.company_street ?? f.your_street,
        your_city: s.company_city ?? f.your_city,
        your_zip: s.company_zip ?? f.your_zip,
        your_country: s.company_country ?? f.your_country,
        your_registration_no: s.registration_no ?? f.your_registration_no,
        your_vat_no: s.vat_no ?? f.your_vat_no,
        bank_account: s.bank_account ?? f.bank_account,
        iban: s.iban ?? f.iban,
        swift_bic: s.swift_bic ?? f.swift_bic,
        // Don't overwrite prefilled client payment prefs
        payment_method: f.payment_method !== 'bank' ? f.payment_method : (s.default_payment_method as PaymentMethod) ?? f.payment_method,
        due: f.due !== 14 ? f.due : s.default_due ?? f.due,
        currency: s.default_currency ?? f.currency,
        vat_exempt: s.vat_exempt ?? f.vat_exempt,
      }))
    }).catch(() => {})
  }, [])

  const set = <K extends keyof InvoiceInput>(field: K, value: InvoiceInput[K]) =>
    setForm(f => ({ ...f, [field]: value }))

  const handleSubjectSelect = (id: string) => {
    setSelectedSubjectId(id)
    if (id === 'new') { navigate('/subjects/new'); return }
    if (!id) return
    const s = subjects.find(s => String(s.id) === id)
    if (!s) return
    setForm(f => ({
      ...f,
      subject_id: s.id,
      client_name: s.name,
      client_registration_no: s.registration_no ?? '',
      client_vat_no: s.vat_no ?? '',
      client_street: s.street ?? '',
      client_city: s.city ?? '',
      client_zip: s.zip ?? '',
      client_country: s.country ?? 'CZ',
      bank_account: f.bank_account || s.bank_account || '',
      payment_method: s.default_payment_method ?? 'bank',
      due: s.default_due ?? 14,
    }))
  }

  const fetchAres = async (ic: string) => {
    if (ic.length !== 8) return
    setAresLoading(true)
    try {
      const s = await api.ares.lookup(ic)
      setForm(f => ({
        ...f,
        client_name: s.name,
        client_vat_no: s.dic,
        client_street: s.street,
        client_city: s.city,
        client_zip: s.zip,
      }))
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally { setAresLoading(false) }
  }

  const updateLine = (i: number, field: keyof LineInput, value: string | number) =>
    setForm(f => {
      const lines = [...f.lines]
      lines[i] = { ...lines[i], [field]: value }
      return { ...f, lines }
    })

  const addLine = () =>
    setForm(f => ({ ...f, lines: [...f.lines, { ...EMPTY_LINE, position: f.lines.length + 1 }] }))

  const addFromPriceItem = (item: PriceItem) => {
    setForm(f => ({
      ...f,
      lines: [...f.lines, {
        position:      f.lines.length + 1,
        price_item_id: item.id,
        name:          item.name,
        quantity:      '1',
        unit_name:     item.unit_name,
        unit_price_hal: item.unit_price_hal,
        vat_rate_bps:  item.vat_rate_bps,
      }],
    }))
    setShowPriceDialog(false)
    setPriceQuery('')
  }

  const removeLine = (i: number) =>
    setForm(f => ({ ...f, lines: f.lines.filter((_, idx) => idx !== i) }))

  const totals = form.lines.reduce((acc, l) => {
    const qty = parseFloat(l.quantity || '1') || 1
    const base = Math.round(l.unit_price_hal * qty)
    const vat = Math.round(base * (l.vat_rate_bps ?? 0) / 10000)
    return { base: acc.base + base, vat: acc.vat + vat, total: acc.total + base + vat }
  }, { base: 0, vat: 0, total: 0 })

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      if (isEdit && editId) {
        const inv = await api.invoices.update(editId, { ...form, document_type: docType })
        navigate(`/invoices/${inv.id}`)
      } else {
        let number = form.number
        if (invoiceFormat?.id) {
          const res = await api.settings.formats.next(invoiceFormat.id)
          number = res.number
        }
        const inv = await api.invoices.create({ ...form, number, document_type: docType })
        navigate(`/invoices/${inv.id}`)
      }
    } catch (err) {
      toast((err as Error).message, 'error')
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col min-h-screen bg-white">
      {/* ── Top bar ─────────────────────────────────────────────────── */}
      <div className="flex items-center px-8 py-4 border-b border-slate-200">
        <h1 className="text-xl font-semibold text-slate-900 flex-1">{isEdit ? 'Upravit fakturu' : 'Nová faktura'}</h1>

        {/* Doc type toggle */}
        <div className="flex rounded-md border border-slate-300 overflow-hidden">
          {(['invoice', 'proforma'] as const).map((type, i) => (
            <button
              key={type}
              type="button"
              onClick={() => setDocType(type)}
              className={`px-4 py-1.5 text-sm font-medium transition-colors ${
                docType === type
                  ? 'bg-white text-slate-900 border-violet-500 border-b-2'
                  : 'bg-white text-slate-500 hover:text-slate-700'
              } ${i > 0 ? 'border-l border-slate-300' : ''}`}
            >
              {type === 'invoice' ? 'Faktura' : 'Zálohovka'}
            </button>
          ))}
        </div>

        <div className="flex-1 flex justify-end">
          <button
            type="button"
            onClick={() => navigate('/invoices')}
            className="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-700 transition-colors"
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
            </svg>
            Zpět
          </button>
        </div>
      </div>

      {/* ── Form body ───────────────────────────────────────────────── */}
      <form ref={formRef} onSubmit={submit} className="flex-1 overflow-y-auto">
        <div className="max-w-2xl mx-auto px-4 py-8 space-y-1">

          {/* Odběratel */}
          <Row label="Odběratel">
            <div className="flex items-center gap-2">
              <select
                value={selectedSubjectId}
                onChange={e => handleSubjectSelect(e.target.value)}
                className="flex-1 h-9 rounded-md border border-slate-300 px-3 text-sm bg-white focus:border-violet-500 focus:outline-none focus:ring-1 focus:ring-violet-500 text-slate-700"
              >
                <option value="">Vyberte odběratele…</option>
                {subjects.map(s => (
                  <option key={s.id} value={String(s.id)}>{s.name}</option>
                ))}
              </select>
              <button
                type="button"
                onClick={() => navigate('/subjects/new')}
                className="flex items-center gap-1 text-sm text-emerald-600 hover:text-emerald-700 whitespace-nowrap font-medium transition-colors"
              >
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                </svg>
                Nový odběratel
              </button>
            </div>
          </Row>

          {/* IČO + ARES (when no subject selected) */}
          {!selectedSubjectId && (
            <Row label="IČO odběratele">
              <div className="flex gap-2">
                <Input
                  value={form.client_registration_no ?? ''}
                  onChange={e => set('client_registration_no', e.target.value)}
                  maxLength={8}
                  placeholder="12345678"
                  className="w-36"
                />
                <button
                  type="button"
                  disabled={aresLoading}
                  onClick={() => fetchAres(form.client_registration_no ?? '')}
                  className="text-sm text-violet-600 hover:text-violet-800 disabled:opacity-40 font-medium transition-colors"
                >
                  {aresLoading ? 'Načítám…' : 'Doplnit z ARES'}
                </button>
              </div>
            </Row>
          )}

          {/* Client name (only when no subject) */}
          {!selectedSubjectId && (
            <Row label="Název odběratele" required>
              <Input
                required
                value={form.client_name}
                onChange={e => set('client_name', e.target.value)}
                placeholder="ACME s.r.o."
              />
            </Row>
          )}

          <div className="h-4" />

          {/* Invoice number */}
          <Row label="Číslo faktury">
            <div className="flex items-center gap-2 pt-1.5">
              <span className="text-sm font-medium text-slate-800">{form.number || '—'}</span>
              {!invoiceFormat && (
                <button
                  type="button"
                  onClick={() => {
                    const n = prompt('Číslo faktury:', form.number)
                    if (n) set('number', n)
                  }}
                  className="text-xs text-violet-600 underline hover:text-violet-800"
                >
                  Změnit
                </button>
              )}
              {invoiceFormat && (
                <span className="text-xs text-slate-400">
                  (generováno z řady „{invoiceFormat.label || invoiceFormat.pattern}")
                </span>
              )}
            </div>
          </Row>

          {/* Issued on */}
          <Row label="Datum vystavení">
            <DateInput
              value={form.issued_on}
              onChange={v => set('issued_on', v)}
              className="w-36"
            />
          </Row>

          {/* Payment method */}
          <Row label="Platební metoda">
            <div className="flex rounded-md border border-slate-300 overflow-hidden w-fit">
              {PAYMENT_METHODS.map((m, i) => (
                <button
                  key={m.key}
                  type="button"
                  onClick={() => set('payment_method', m.key)}
                  className={`px-3 py-1.5 text-sm transition-colors ${
                    form.payment_method === m.key
                      ? 'bg-white text-violet-700 font-semibold border-b-2 border-violet-500'
                      : 'bg-white text-slate-600 hover:text-slate-900'
                  } ${i > 0 ? 'border-l border-slate-300' : ''}`}
                >
                  {m.label}
                </button>
              ))}
            </div>
          </Row>

          {/* Due */}
          <Row label="Splatnost">
            <div className="flex items-center gap-3">
              <select
                value={form.due}
                onChange={e => set('due', Number(e.target.value))}
                className="h-9 rounded-md border border-slate-300 px-3 text-sm bg-white focus:border-violet-500 focus:outline-none focus:ring-1 focus:ring-violet-500"
              >
                {DUE_OPTIONS.map(d => (
                  <option key={d} value={d}>{d === 0 ? 'Ihned' : `${d} dní`}</option>
                ))}
                {!DUE_OPTIONS.includes(form.due ?? 14) && (
                  <option value={form.due ?? 14}>{form.due ?? 14} dní</option>
                )}
              </select>
              <span className="text-sm text-slate-400">
                (vychází na {dueDate(form.issued_on, form.due ?? 14)})
              </span>
            </div>
          </Row>

          {/* VAT exempt */}
          <Row label="DPH">
            <label className="flex items-center gap-2 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={form.vat_exempt ?? true}
                onChange={e => {
                  const exempt = e.target.checked
                  setForm(f => ({
                    ...f,
                    vat_exempt: exempt,
                    lines: f.lines.map(l => ({ ...l, vat_rate_bps: exempt ? 0 : l.vat_rate_bps })),
                  }))
                }}
                className="h-4 w-4 rounded border-slate-300 text-violet-600 focus:ring-violet-500"
              />
              <span className="text-sm text-slate-700">Nejsem plátce DPH</span>
            </label>
          </Row>

          {/* Currency */}
          <Row label="Měna">
            <select
              value={form.currency ?? 'CZK'}
              onChange={e => set('currency', e.target.value)}
              className="h-9 w-24 rounded-md border border-slate-300 px-3 text-sm bg-white focus:border-violet-500 focus:outline-none focus:ring-1 focus:ring-violet-500"
            >
              {['CZK', 'EUR', 'USD'].map(c => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </Row>

          {/* Další možnosti */}
          <div className="pt-2">
            <button
              type="button"
              onClick={() => setShowMore(v => !v)}
              className="flex items-center gap-1.5 text-sm text-slate-500 hover:text-slate-700 transition-colors ml-44"
            >
              <svg className={`h-4 w-4 transition-transform ${showMore ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <circle cx="12" cy="12" r="9" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v8M8 12h8" />
              </svg>
              Další možnosti
            </button>

            {showMore && (
              <div className="mt-3 space-y-1">
                <Row label="DUZP">
                  <DateInput
                    value={form.taxable_fulfillment_due}
                    onChange={v => set('taxable_fulfillment_due', v)}
                    className="w-36"
                  />
                </Row>
                <Row label="Variabilní symbol">
                  <Input
                    value={form.variable_symbol ?? ''}
                    onChange={e => set('variable_symbol', e.target.value)}
                    className="w-48"
                  />
                </Row>
                <Row label="Bankovní účet">
                  <Input
                    value={form.bank_account ?? ''}
                    onChange={e => {
                      const val = e.target.value
                      setForm(f => ({ ...f, bank_account: val, iban: czIban(val) || '', swift_bic: czSwift(val) || '' }))
                    }}
                    placeholder="123456789/0800"
                    className="w-56"
                  />
                </Row>
                <Row label="IBAN">
                  <Input
                    value={form.iban ?? ''}
                    onChange={e => set('iban', e.target.value)}
                    placeholder="CZ6508000000192000145399"
                    className="w-72"
                  />
                </Row>
                <Row label="SWIFT / BIC">
                  <Input
                    value={form.swift_bic ?? ''}
                    onChange={e => set('swift_bic', e.target.value)}
                    placeholder="GIBACZPX"
                    className="w-48"
                  />
                </Row>
                <Row label="Poznámka">
                  <Textarea
                    value={form.note ?? ''}
                    onChange={e => set('note', e.target.value)}
                    rows={2}
                    className="resize-none"
                  />
                </Row>
              </div>
            )}
          </div>
        </div>

        {/* ── Items table ─────────────────────────────────────────────── */}
        <div className="border-t border-slate-200 bg-slate-50/50">
          <div className="max-w-5xl mx-auto px-8 py-6">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs font-semibold text-slate-400 uppercase tracking-wide">
                  <th className="pb-3 text-left w-8" />
                  <th className="pb-3 text-left w-20 pr-2">Počet</th>
                  <th className="pb-3 text-left w-20 pr-2">MJ</th>
                  <th className="pb-3 text-left pr-2">Popis</th>
                  <th className="pb-3 text-right w-36">Cena za MJ</th>
                  {!form.vat_exempt && <th className="pb-3 text-right w-24">Sazba DPH</th>}
                  <th className="pb-3 w-8" />
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-200">
                {form.lines.map((line, i) => {
                  return (
                    <tr key={i} className="group">
                      {/* drag handle placeholder */}
                      <td className="py-2 pr-2">
                        <svg className="h-4 w-4 text-slate-300 group-hover:text-slate-400 cursor-grab" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 8h16M4 16h16" />
                        </svg>
                      </td>
                      <td className="py-2 pr-2">
                        <Input
                          type="number"
                          min="0.001"
                          step="0.001"
                          value={line.quantity}
                          onChange={e => updateLine(i, 'quantity', e.target.value)}
                          className="w-20"
                        />
                      </td>
                      <td className="py-2 pr-2">
                        <Input
                          value={line.unit_name}
                          onChange={e => updateLine(i, 'unit_name', e.target.value)}
                          placeholder="ks, …"
                          className="w-20"
                        />
                      </td>
                      <td className="py-2 pr-2">
                        <Input
                          value={line.name}
                          onChange={e => updateLine(i, 'name', e.target.value)}
                          required
                          placeholder="Popis služby nebo zboží"
                          className="w-full"
                        />
                      </td>
                      <td className="py-2 text-right">
                        <Input
                          type="number"
                          min="0"
                          step="0.01"
                          value={line.unit_price_hal / 100}
                          onChange={e => updateLine(i, 'unit_price_hal', Math.round(parseFloat(e.target.value) * 100))}
                          className="w-28 text-right"
                        />
                      </td>
                      {!form.vat_exempt && (
                        <td className="py-2 pl-2 text-right">
                          <select
                            value={line.vat_rate_bps}
                            onChange={e => updateLine(i, 'vat_rate_bps', Number(e.target.value))}
                            className="h-9 rounded-md border border-slate-300 px-2 text-sm bg-white focus:border-violet-500 focus:outline-none focus:ring-1 focus:ring-violet-500"
                          >
                            <option value={0}>0 %</option>
                            <option value={1200}>12 %</option>
                            <option value={2100}>21 %</option>
                          </select>
                        </td>
                      )}
                      <td className="py-2 pl-2">
                        {form.lines.length > 1 && (
                          <button
                            type="button"
                            onClick={() => removeLine(i)}
                            className="h-6 w-6 rounded-full flex items-center justify-center text-slate-300 hover:text-red-400 hover:bg-red-50 transition-colors"
                          >
                            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                            </svg>
                          </button>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>

            <div className="flex gap-4 mt-3">
              <button
                type="button"
                onClick={addLine}
                className="flex items-center gap-1.5 text-sm text-emerald-600 hover:text-emerald-700 font-medium transition-colors"
              >
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                </svg>
                Přidat položku
              </button>
              {priceItems.length > 0 && (
                <button
                  type="button"
                  onClick={() => setShowPriceDialog(true)}
                  className="flex items-center gap-1.5 text-sm text-violet-600 hover:text-violet-700 font-medium transition-colors"
                >
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                  </svg>
                  Z ceníku
                </button>
              )}
            </div>
          </div>
        </div>

        {/* ── Totals bar ──────────────────────────────────────────────── */}
        <div className="border-t border-slate-200 bg-slate-50">
          <div className="max-w-5xl mx-auto px-8 py-4 flex justify-end">
            <div className="w-56 space-y-1 text-sm">
              {!form.vat_exempt && (() => {
                // group by vat rate
                const byRate = new Map<number, { base: number; vat: number }>()
                for (const l of form.lines) {
                  const qty = parseFloat(l.quantity || '1') || 1
                  const base = Math.round(l.unit_price_hal * qty)
                  const rate = l.vat_rate_bps ?? 0
                  const vat = Math.round(base * rate / 10000)
                  const g = byRate.get(rate) ?? { base: 0, vat: 0 }
                  byRate.set(rate, { base: g.base + base, vat: g.vat + vat })
                }
                return [...byRate.entries()].sort((a, b) => b[0] - a[0]).map(([bps, g]) => (
                  <div key={bps}>
                    <div className="flex justify-between text-slate-500">
                      <span>Základ {bps / 100} %</span><span>{formatKc(g.base)}</span>
                    </div>
                    <div className="flex justify-between text-slate-500">
                      <span>DPH {bps / 100} %</span><span>{formatKc(g.vat)}</span>
                    </div>
                  </div>
                ))
              })()}
              <div className="flex justify-between font-semibold text-slate-900 text-base border-t border-violet-300 pt-2">
                <span>Celkem</span>
                <span>{formatKc(totals.total)}</span>
              </div>
            </div>
          </div>
        </div>

      </form>

      {/* ── Dialog: Z ceníku ────────────────────────────────────────── */}
      {showPriceDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setShowPriceDialog(false)}>
          <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 flex flex-col max-h-[80vh]" onClick={e => e.stopPropagation()}>
            {/* Header */}
            <div className="flex items-center justify-between px-5 py-4 border-b border-slate-200">
              <h2 className="text-base font-semibold text-slate-900">Přidat z ceníku</h2>
              <button onClick={() => setShowPriceDialog(false)} className="text-slate-400 hover:text-slate-700">
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            {/* Search */}
            <div className="px-5 py-3 border-b border-slate-100">
              <input
                autoFocus
                type="text"
                placeholder="Hledat název, kat. číslo…"
                value={priceQuery}
                onChange={e => setPriceQuery(e.target.value)}
                className="w-full rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-violet-500 focus:outline-none focus:ring-1 focus:ring-violet-500"
              />
            </div>
            {/* List */}
            <div className="overflow-y-auto flex-1">
              {priceItems
                .filter(i => !i.archived && (
                  !priceQuery ||
                  i.name.toLowerCase().includes(priceQuery.toLowerCase()) ||
                  (i.catalog_no ?? '').toLowerCase().includes(priceQuery.toLowerCase())
                ))
                .map(item => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => addFromPriceItem(item)}
                    className="w-full flex items-center justify-between px-5 py-3 hover:bg-slate-50 transition-colors border-b border-slate-50 text-left"
                  >
                    <div>
                      <p className="text-sm font-medium text-slate-900">{item.name}</p>
                      {item.catalog_no && (
                        <p className="text-xs text-slate-400 mt-0.5">{item.catalog_no}</p>
                      )}
                    </div>
                    <div className="text-right ml-4 shrink-0">
                      <p className="text-sm font-mono text-slate-800">
                        {(item.unit_price_hal / 100).toFixed(2)} Kč
                      </p>
                      <p className="text-xs text-slate-400">
                        {item.vat_rate_bps === 0 ? 'bez DPH' : `+ ${item.vat_rate_bps / 100} % DPH`} / {item.unit_name}
                      </p>
                      {item.track_stock && (
                        <p className={`text-xs mt-0.5 ${(item.stock_quantity ?? 0) < 0 ? 'text-red-500' : 'text-emerald-600'}`}>
                          sklad: {item.stock_quantity ?? 0} {item.unit_name}
                        </p>
                      )}
                    </div>
                  </button>
                ))}
            </div>
          </div>
        </div>
      )}

      {/* ── Sticky footer ───────────────────────────────────────────── */}
      <div className="sticky bottom-0 border-t border-slate-200 bg-white px-8 py-4 flex items-center justify-end gap-3 shadow-[0_-1px_8px_oklch(0_0_0/0.06)]">
        <button
          type="button"
          onClick={() => navigate('/invoices')}
          className="text-sm text-slate-500 hover:text-slate-700 transition-colors px-2"
        >
          Zrušit
        </button>
        <button
          type="button"
          disabled={saving}
          onClick={() => formRef.current?.requestSubmit()}
          className="text-sm text-slate-600 hover:text-slate-800 border border-slate-300 px-4 py-2 rounded-md transition-colors disabled:opacity-40"
        >
          Uložit jako koncept
        </button>
        <button
          type="submit"
          form=""
          disabled={saving}
          onClick={submit}
          className="bg-amber-400 hover:bg-amber-500 text-slate-900 font-semibold text-sm px-6 py-2 rounded-md transition-colors disabled:opacity-40 shadow-sm"
        >
          {saving ? 'Ukládám…' : isEdit ? 'Uložit změny' : 'Vytvořit fakturu'}
        </button>
      </div>
    </div>
  )
}
