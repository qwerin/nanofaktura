import { useMemo, useState } from 'react'
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import type { Invoice } from '../api/client'

type Period = 'month' | 'quarter' | 'year'

const PERIOD_TABS: { key: Period; label: string }[] = [
  { key: 'month',   label: 'Měsíce' },
  { key: 'quarter', label: 'Kvartály' },
  { key: 'year',    label: 'Roky' },
]

function periodKey(date: Date, period: Period): string {
  const y = date.getFullYear()
  const m = date.getMonth() + 1
  if (period === 'year')    return `${y}`
  if (period === 'quarter') return `${y}-Q${Math.ceil(m / 3)}`
  return `${y}-${String(m).padStart(2, '0')}`
}

function periodLabel(key: string, period: Period): string {
  if (period === 'year') return key
  if (period === 'quarter') {
    const [y, q] = key.split('-')
    return `${q} ${y}`
  }
  const [y, m] = key.split('-')
  const months = ['Led','Úno','Bře','Dub','Kvě','Čer','Čvc','Srp','Zář','Říj','Lis','Pro']
  return `${months[parseInt(m) - 1]} ${y}`
}

function buckets(period: Period): string[] {
  const now = new Date()
  const result: string[] = []
  if (period === 'year') {
    for (let i = 4; i >= 0; i--) {
      result.push(String(now.getFullYear() - i))
    }
  } else if (period === 'quarter') {
    for (let i = 7; i >= 0; i--) {
      const d = new Date(now.getFullYear(), now.getMonth() - i * 3, 1)
      result.push(periodKey(d, 'quarter'))
    }
  } else {
    for (let i = 11; i >= 0; i--) {
      const d = new Date(now.getFullYear(), now.getMonth() - i, 1)
      result.push(periodKey(d, 'month'))
    }
  }
  return result
}

function formatKcShort(hal: number): string {
  const kc = hal / 100
  if (kc >= 1_000_000) return `${(kc / 1_000_000).toFixed(1)} M`
  if (kc >= 1_000)     return `${(kc / 1_000).toFixed(0)} tis`
  return `${kc.toFixed(0)}`
}

function formatKc(hal: number): string {
  return new Intl.NumberFormat('cs-CZ', { style: 'currency', currency: 'CZK', maximumFractionDigits: 0 }).format(hal / 100)
}

export function RevenueChart({ invoices }: { invoices: Invoice[] }) {
  const [period, setPeriod] = useState<Period>('month')

  const data = useMemo(() => {
    const keys = buckets(period)
    const map: Record<string, { issued: number; paid: number }> = {}
    for (const k of keys) map[k] = { issued: 0, paid: 0 }

    for (const inv of invoices) {
      const d = new Date(inv.issued_on)
      if (isNaN(d.getTime())) continue
      const k = periodKey(d, period)
      if (!(k in map)) continue
      map[k].issued += inv.total ?? 0
      if (inv.status === 'paid') map[k].paid += inv.total ?? 0
    }

    return keys.map(k => ({
      label: periodLabel(k, period),
      issued: map[k].issued,
      paid: map[k].paid,
    }))
  }, [invoices, period])

  const total = data.reduce((s, d) => s + d.issued, 0)
  const totalPaid = data.reduce((s, d) => s + d.paid, 0)

  return (
    <div className="bg-white rounded-xl border border-slate-200 shadow-sm p-5 mb-6">
      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div>
          <p className="text-xs font-medium text-slate-500 uppercase tracking-wide">Obrat</p>
          <p className="text-2xl font-semibold text-slate-900 mt-0.5">{formatKc(total)}</p>
          <p className="text-xs text-slate-400 mt-0.5">
            zaplaceno <span className="text-emerald-600 font-medium">{formatKc(totalPaid)}</span>
          </p>
        </div>

        {/* Period toggle */}
        <div className="flex gap-0.5 rounded-lg bg-slate-100 p-0.5">
          {PERIOD_TABS.map(tab => (
            <button
              key={tab.key}
              onClick={() => setPeriod(tab.key)}
              className={`px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
                period === tab.key
                  ? 'bg-white text-slate-900 shadow-sm'
                  : 'text-slate-500 hover:text-slate-700'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Chart */}
      <div className="h-44">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 4, right: 4, left: 0, bottom: 0 }} barCategoryGap="28%">
            <CartesianGrid strokeDasharray="3 3" stroke="oklch(0.94 0 0)" vertical={false} />
            <XAxis
              dataKey="label"
              tick={{ fontSize: 11, fill: 'oklch(0.55 0 0)' }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={formatKcShort}
              tick={{ fontSize: 11, fill: 'oklch(0.55 0 0)' }}
              tickLine={false}
              axisLine={false}
              width={52}
            />
            <Tooltip
              formatter={(val: number, name: string) => [
                formatKc(val),
                name === 'issued' ? 'Vydáno' : 'Zaplaceno',
              ]}
              contentStyle={{
                fontSize: 12,
                border: '1px solid oklch(0.92 0 0)',
                borderRadius: 8,
                boxShadow: '0 4px 12px oklch(0 0 0 / 0.08)',
              }}
              cursor={{ fill: 'oklch(0.97 0 0)' }}
            />
            <Bar dataKey="issued" fill="oklch(0.585 0.233 277.1)" radius={[3, 3, 0, 0]} />
            <Bar dataKey="paid"   fill="oklch(0.696 0.17 162.5)"  radius={[3, 3, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>

      {/* Legend */}
      <div className="flex gap-4 mt-3">
        <span className="flex items-center gap-1.5 text-xs text-slate-500">
          <span className="w-3 h-0.5 rounded bg-violet-500 inline-block" />
          Vydáno
        </span>
        <span className="flex items-center gap-1.5 text-xs text-slate-500">
          <span className="w-3 h-0.5 rounded bg-emerald-500 inline-block" />
          Zaplaceno
        </span>
      </div>
    </div>
  )
}
