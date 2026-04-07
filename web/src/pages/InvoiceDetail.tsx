import { useEffect, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { api, type Invoice, type InvoiceStatus } from '../api/client'
import { StatusBadge } from '../components/StatusBadge'
import { formatKc, formatQty } from '../utils/money'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

export function InvoiceDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [inv, setInv] = useState<Invoice | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    api.invoices.get(Number(id))
      .then(setInv)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) return (
    <div className="flex items-center justify-center py-24 text-slate-400">
      <svg className="animate-spin h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
      Načítám…
    </div>
  )

  if (error || !inv) return (
    <div className="p-4 md:p-8">
      <div className="rounded-lg bg-red-50 border border-red-200 p-4 text-sm text-red-700">
        Chyba: {error ?? 'Faktura nenalezena'}
      </div>
    </div>
  )

  return (
    <div className="p-4 md:p-8 max-w-4xl">
      {/* Page header */}
      <div className="flex flex-col gap-4 mb-6 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl md:text-2xl font-semibold text-slate-900">Faktura {inv.number}</h1>
            <StatusBadge status={(inv.status ?? 'open') as InvoiceStatus} />
          </div>
          <p className="mt-1 text-sm text-slate-500">
            Vystaveno: {inv.issued_on} · Splatnost: {inv.due} dní od vystavení
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            onClick={() => navigate(`/invoices/${inv.id}/edit`)}
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
            Upravit
          </Button>
          <Button
            variant="outline"
            onClick={() => navigate('/invoices/new', { state: { prefill: inv } })}
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
            Duplikovat
          </Button>
          <a
            href={`/api/invoices/${inv.id}/pdf`}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(buttonVariants({ variant: 'outline' }), 'gap-2')}
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            Stáhnout PDF
          </a>
          <Button variant="outline" asChild>
            <Link to="/invoices">← Zpět</Link>
          </Button>
        </div>
      </div>

      {/* Invoice card — print-preview style */}
      <Card className="overflow-hidden">
        {/* Parties */}
        <CardHeader className="p-0">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-0 sm:divide-x divide-slate-100 border-b border-slate-100">
            <div className="p-6">
              <p className="text-xs font-semibold uppercase tracking-wide text-slate-400 mb-3">Dodavatel</p>
              <p className="font-semibold text-slate-900">{inv.your_name}</p>
              {inv.your_registration_no && <p className="text-sm text-slate-500 mt-1">IČO: {inv.your_registration_no}</p>}
              {inv.your_vat_no && <p className="text-sm text-slate-500">DIČ: {inv.your_vat_no}</p>}
              {inv.your_street && <p className="text-sm text-slate-600 mt-2">{inv.your_street}</p>}
              {(inv.your_zip || inv.your_city) && (
                <p className="text-sm text-slate-600">{inv.your_zip} {inv.your_city}</p>
              )}
            </div>
            <div className="p-6">
              <p className="text-xs font-semibold uppercase tracking-wide text-slate-400 mb-3">Odběratel</p>
              <p className="font-semibold text-slate-900">{inv.client_name}</p>
              {inv.client_registration_no && <p className="text-sm text-slate-500 mt-1">IČO: {inv.client_registration_no}</p>}
              {inv.client_vat_no && <p className="text-sm text-slate-500">DIČ: {inv.client_vat_no}</p>}
              {inv.client_street && <p className="text-sm text-slate-600 mt-2">{inv.client_street}</p>}
              {(inv.client_zip || inv.client_city) && (
                <p className="text-sm text-slate-600">{inv.client_zip} {inv.client_city}</p>
              )}
            </div>
          </div>
        </CardHeader>

        <CardContent className="p-0">
          {/* Items table */}
          <Table>
            <TableHeader className="bg-slate-50">
              <TableRow>
                <TableHead>Popis</TableHead>
                <TableHead className="text-right">Množství</TableHead>
                <TableHead className="text-right">Cena/jed.</TableHead>
                <TableHead className="text-right">Celkem</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {inv.lines?.map((line, i) => (
                <TableRow key={i}>
                  <TableCell className="text-slate-800">{line.name}</TableCell>
                  <TableCell className="text-right text-slate-600">
                    {formatQty(line.quantity)} {line.unit_name}
                  </TableCell>
                  <TableCell className="text-right text-slate-600">{formatKc(line.unit_price_hal)}</TableCell>
                  <TableCell className="text-right font-medium text-slate-900">{formatKc(line.total_hal ?? 0)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          {/* Totals */}
          <div className="flex justify-end bg-slate-50 px-4 md:px-6 py-4">
            <div className="w-full sm:w-64 space-y-1.5 text-sm">
              {!inv.vat_exempt && (
                <>
                  <div className="flex justify-between text-slate-500">
                    <span>Základ DPH</span>
                    <span>{formatKc(inv.subtotal ?? 0)}</span>
                  </div>
                  <div className="flex justify-between text-slate-500">
                    <span>DPH</span>
                    <span>{formatKc(inv.total_vat_hal ?? 0)}</span>
                  </div>
                  <Separator />
                </>
              )}
              <div className="flex justify-between font-bold text-slate-900 text-base">
                <span>Celkem k úhradě</span>
                <span>{formatKc(inv.total ?? 0)}</span>
              </div>
            </div>
          </div>

          {/* Payment info */}
          {(inv.bank_account || inv.variable_symbol) && (
            <>
              <Separator />
              <div className="px-4 md:px-6 py-4 text-sm text-slate-600 flex flex-wrap gap-4 md:gap-6">
                {inv.bank_account && (
                  <span><span className="font-medium text-slate-700">Číslo účtu:</span> {inv.bank_account}</span>
                )}
                {inv.variable_symbol && (
                  <span><span className="font-medium text-slate-700">VS:</span> {inv.variable_symbol}</span>
                )}
                {inv.iban && (
                  <span><span className="font-medium text-slate-700">IBAN:</span> {inv.iban}</span>
                )}
              </div>
            </>
          )}

          {inv.vat_exempt && (
            <>
              <Separator />
              <div className="px-6 py-3 text-xs text-slate-400">
                Fyzická osoba není plátcem DPH.
              </div>
            </>
          )}

          {inv.note && (
            <>
              <Separator />
              <div className="px-6 py-4 text-sm text-slate-600">
                <span className="font-medium text-slate-700">Poznámka:</span> {inv.note}
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
