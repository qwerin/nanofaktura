import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type Subject, type SubjectType } from '../api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const TYPE_LABELS: Record<SubjectType, string> = {
  customer: 'Zákazník',
  supplier: 'Dodavatel',
  both:     'Obojí',
}

export function SubjectList() {
  const [subjects, setSubjects] = useState<Subject[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  useEffect(() => {
    api.subjects.list()
      .then(setSubjects)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const filtered = query.trim()
    ? subjects.filter(s =>
        s.name.toLowerCase().includes(query.toLowerCase()) ||
        (s.registration_no ?? '').includes(query)
      )
    : subjects

  const handleDelete = async (id: number) => {
    if (!confirm('Opravdu smazat kontakt?')) return
    try {
      await api.subjects.delete(id)
      setSubjects(prev => prev.filter(s => s.id !== id))
    } catch (e) {
      alert((e as Error).message)
    }
  }

  return (
    <div className="p-8">
      {/* Page header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Kontakty</h1>
          <p className="mt-1 text-sm text-slate-500">Zákazníci a dodavatelé</p>
        </div>
        <Button asChild>
          <Link to="/subjects/new">
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Nový kontakt
          </Link>
        </Button>
      </div>

      {/* Search */}
      <div className="mb-5">
        <input
          type="search"
          placeholder="Hledat podle jména nebo IČO…"
          value={query}
          onChange={e => setQuery(e.target.value)}
          className="w-full max-w-sm rounded-lg border border-slate-200 px-3 py-2 text-sm shadow-sm focus:border-violet-500 focus:outline-none focus:ring-1 focus:ring-violet-500 bg-white"
        />
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

      {error && (
        <div className="rounded-lg bg-red-50 border border-red-200 p-4 text-sm text-red-700">
          Chyba: {error}
        </div>
      )}

      {!loading && !error && filtered.length === 0 && (
        <div className="flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-slate-200 py-16 text-center">
          <svg className="h-12 w-12 text-slate-300 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          <p className="text-slate-500 font-medium">
            {query ? 'Žádné výsledky' : 'Zatím žádné kontakty'}
          </p>
          <p className="mt-1 text-sm text-slate-400">
            {query ? 'Zkuste jiný hledaný výraz.' : 'Přidejte svého prvního zákazníka nebo dodavatele.'}
          </p>
          {!query && (
            <Button asChild className="mt-4">
              <Link to="/subjects/new">Přidat kontakt</Link>
            </Button>
          )}
        </div>
      )}

      {!loading && !error && filtered.length > 0 && (
        <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-slate-50">
                <TableHead>Název</TableHead>
                <TableHead>IČO</TableHead>
                <TableHead>DIČ</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Stav</TableHead>
                <TableHead>Akce</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map(s => (
                <TableRow key={s.id}>
                  <TableCell className="font-medium text-slate-900">{s.name}</TableCell>
                  <TableCell className="font-mono text-slate-500">{s.registration_no || '—'}</TableCell>
                  <TableCell className="font-mono text-slate-500">{s.vat_no || '—'}</TableCell>
                  <TableCell className="text-slate-500">{s.email || '—'}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">
                      {TYPE_LABELS[s.type ?? 'customer'] ?? s.type}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => s.id && handleDelete(s.id)}
                      className="text-slate-400 hover:text-red-500"
                    >
                      Smazat
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
