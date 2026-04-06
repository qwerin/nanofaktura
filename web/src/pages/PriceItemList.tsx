import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type PriceItem } from '../api/client'
import { toast } from '../components/Toast'
import { formatKc } from '../utils/money'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function PriceItemList() {
  const navigate = useNavigate()
  const [items, setItems] = useState<PriceItem[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [showArchived, setShowArchived] = useState(false)

  const load = (archived = showArchived) => {
    setLoading(true)
    api.priceItems.list(archived)
      .then(setItems)
      .catch((e: Error) => toast(e.message, 'error'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const filtered = query.trim()
    ? items.filter(i =>
        i.name.toLowerCase().includes(query.toLowerCase()) ||
        (i.catalog_no ?? '').toLowerCase().includes(query.toLowerCase()) ||
        (i.ean ?? '').includes(query)
      )
    : items

  const toggleArchived = () => {
    const next = !showArchived
    setShowArchived(next)
    load(next)
  }

  return (
    <div className="p-8">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Ceník</h1>
          <p className="mt-1 text-sm text-slate-500">Položky ceníku a sklad zásob.</p>
        </div>
        <Link to="/price-items/new">
          <Button>+ Nová položka</Button>
        </Link>
      </div>

      <div className="flex items-center gap-3 mb-4">
        <Input
          placeholder="Hledat název, kat. číslo, EAN…"
          value={query}
          onChange={e => setQuery(e.target.value)}
          className="max-w-xs"
        />
        <button
          onClick={toggleArchived}
          className={`text-xs font-medium px-3 py-1.5 rounded-full border transition-colors ${
            showArchived
              ? 'bg-slate-800 text-white border-slate-800'
              : 'text-slate-500 border-slate-200 hover:border-slate-400'
          }`}
        >
          {showArchived ? 'Skrýt archivované' : 'Zobrazit archivované'}
        </button>
      </div>

      {loading && (
        <div className="flex items-center gap-2 text-sm text-slate-400 py-8">
          <svg className="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          Načítám…
        </div>
      )}

      {!loading && filtered.length === 0 && (
        <div className="flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-slate-200 py-16 text-center">
          <svg className="h-12 w-12 text-slate-300 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
          </svg>
          <p className="text-slate-500 font-medium">{query ? 'Žádné výsledky' : 'Ceník je prázdný'}</p>
          {!query && (
            <p className="mt-1 text-sm text-slate-400">Přidejte první položku ceníku.</p>
          )}
        </div>
      )}

      {!loading && filtered.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white overflow-hidden">
          <Table>
            <TableHeader className="bg-slate-50">
              <TableRow>
                <TableHead>Název</TableHead>
                <TableHead>Kat. číslo</TableHead>
                <TableHead>EAN</TableHead>
                <TableHead className="text-right">Cena bez DPH</TableHead>
                <TableHead className="text-right">DPH</TableHead>
                <TableHead>Jednotka</TableHead>
                <TableHead className="text-right">Sklad</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map(item => (
                <TableRow
                  key={item.id}
                  className={`cursor-pointer hover:bg-slate-50 ${item.archived ? 'opacity-50' : ''}`}
                  onClick={() => navigate(`/price-items/${item.id}`)}
                >
                  <TableCell className="font-medium text-slate-900">
                    {item.name}
                    {item.archived && (
                      <Badge className="ml-2 bg-slate-100 text-slate-500 text-xs">Archivováno</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-slate-500 text-sm">{item.catalog_no ?? '—'}</TableCell>
                  <TableCell className="text-slate-500 text-sm font-mono text-xs">{item.ean ?? '—'}</TableCell>
                  <TableCell className="text-right font-mono text-sm">{formatKc(item.unit_price_hal)}</TableCell>
                  <TableCell className="text-right text-sm text-slate-500">
                    {item.vat_rate_bps === 0 ? '0 %' : `${item.vat_rate_bps / 100} %`}
                  </TableCell>
                  <TableCell className="text-sm text-slate-500">{item.unit_name}</TableCell>
                  <TableCell className="text-right text-sm">
                    {item.track_stock ? (
                      <span className={`font-mono font-medium ${(item.stock_quantity ?? 0) < 0 ? 'text-red-600' : 'text-slate-800'}`}>
                        {item.stock_quantity ?? 0} {item.unit_name}
                      </span>
                    ) : (
                      <span className="text-slate-300">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right" onClick={e => e.stopPropagation()}>
                    <Button variant="ghost" size="sm" asChild className="text-slate-400 hover:text-slate-700">
                      <Link to={`/price-items/${item.id}`}>Upravit</Link>
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
