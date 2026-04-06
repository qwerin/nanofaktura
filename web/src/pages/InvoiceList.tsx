import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type Invoice, type InvoiceStatus } from '../api/client'
import { toast } from '../components/Toast'
import { StatusBadge } from '../components/StatusBadge'
import { RevenueChart } from '../components/RevenueChart'
import { formatKc } from '../utils/money'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type Filter = 'all' | InvoiceStatus

const FILTER_TABS: { key: Filter; label: string }[] = [
  { key: 'all',     label: 'Všechny' },
  { key: 'open',    label: 'Neuhrazené' },
  { key: 'overdue', label: 'Po splatnosti' },
  { key: 'paid',    label: 'Uhrazené' },
]

export function InvoiceList() {
  const navigate = useNavigate()
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')

  useEffect(() => {
    api.invoices.list()
      .then(setInvoices)
      .catch((e: Error) => toast(e.message, 'error'))
      .finally(() => setLoading(false))
  }, [])

  const filtered = filter === 'all'
    ? invoices
    : invoices.filter(inv => inv.status === filter)

  return (
    <div className="p-8">
      {/* Page header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Faktury</h1>
          <p className="mt-1 text-sm text-slate-500">Správa vydaných faktur</p>
        </div>
        <Button asChild size="default">
          <Link to="/invoices/new">
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Nová faktura
          </Link>
        </Button>
      </div>

      {!loading && <RevenueChart invoices={invoices} />}

      {/* Filter tabs */}
      <div className="flex gap-1 mb-6 border-b border-slate-200">
        {FILTER_TABS.map(tab => (
          <button
            key={tab.key}
            onClick={() => setFilter(tab.key)}
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
              filter === tab.key
                ? 'border-violet-600 text-violet-600'
                : 'border-transparent text-slate-500 hover:text-slate-700'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {loading && (
        <div className="flex items-center justify-center py-16 text-slate-400">
          <svg className="animate-spin h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          Načítám…
        </div>
      )}

      {!loading && filtered.length === 0 && (
        <div className="flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-slate-200 py-16 text-center">
          <svg className="h-12 w-12 text-slate-300 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
          <p className="text-slate-500 font-medium">Žádné faktury</p>
          <p className="mt-1 text-sm text-slate-400">
            {filter === 'all' ? 'Vytvořte svou první fakturu.' : 'V této kategorii nejsou žádné faktury.'}
          </p>
          {filter === 'all' && (
            <Button asChild className="mt-4">
              <Link to="/invoices/new">Vytvořit fakturu</Link>
            </Button>
          )}
        </div>
      )}

      {!loading && filtered.length > 0 && (
        <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-slate-50">
                <TableHead>Číslo</TableHead>
                <TableHead>Klient</TableHead>
                <TableHead>Vystaveno</TableHead>
                <TableHead>Splatnost (dnů)</TableHead>
                <TableHead className="text-right">Částka</TableHead>
                <TableHead>Stav</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((inv) => (
                <TableRow
                  key={inv.id}
                  className="cursor-pointer hover:bg-slate-50"
                  onClick={() => navigate(`/invoices/${inv.id}`)}
                >
                  <TableCell className="font-mono font-medium text-slate-900">{inv.number}</TableCell>
                  <TableCell className="text-slate-700">{inv.client_name}</TableCell>
                  <TableCell className="text-slate-500">{inv.issued_on}</TableCell>
                  <TableCell className="text-slate-500">{inv.due} dní</TableCell>
                  <TableCell className="text-right font-medium text-slate-900">
                    {formatKc(inv.total ?? 0)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={(inv.status ?? 'open') as InvoiceStatus} />
                  </TableCell>
                  <TableCell className="text-right" onClick={e => e.stopPropagation()}>
                    <Button variant="ghost" size="sm" asChild className="text-slate-400 hover:text-slate-700">
                      <Link to={`/invoices/${inv.id}/edit`}>Upravit</Link>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
