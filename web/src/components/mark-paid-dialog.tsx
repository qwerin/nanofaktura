import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'
import { api, unwrap } from '@/api/client'
import { errorMessage } from '@/api/errors'
import { keys } from '@/api/queries/keys'
import type { ExpenseListFilters, InvoiceListQuery, RequestBody } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { parseISODate, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'

type Body = RequestBody<'/api/accounts/{slug}/invoices/mark-paid', 'post'>

type Target =
  | { kind: 'invoices'; filters: Omit<InvoiceListQuery, 'page' | 'per_page'> }
  | { kind: 'expenses'; filters: ExpenseListFilters }

function markPaid(slug: string, t: Target, body: Body, signal?: AbortSignal) {
  return t.kind === 'invoices'
    ? unwrap(
        api.POST('/api/accounts/{slug}/invoices/mark-paid', {
          params: { path: { slug }, query: t.filters },
          body,
          signal,
        }),
      )
    : unwrap(
        api.POST('/api/accounts/{slug}/expenses/mark-paid', {
          params: { path: { slug }, query: t.filters },
          body,
          signal,
        }),
      )
}

const nouns = {
  invoices: ['faktura', 'faktury', 'faktur'],
  expenses: ['náklad', 'náklady', 'nákladů'],
} as const

function countLabel(kind: Target['kind'], n: number) {
  const [one, few, many] = nouns[kind]
  return `${n} ${n === 1 ? one : n > 1 && n < 5 ? few : many}`
}

/**
 * Hromadně označí všechny neuhrazené doklady aktuálního filtru jako uhrazené
 * (typicky staré doklady zadané zpětně). Nejdřív ukáže, kolik dokladů a za kolik
 * se označí (dry_run); poděkování za úhradu se neposílá.
 */
export function MarkPaidDialog({
  slug,
  target,
  filtered,
  open,
  onOpenChange,
}: {
  slug: string
  target: Target
  /** Je aktivní nějaký filtr (jinak upozorníme, že jde o všechny doklady). */
  filtered: boolean
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const qc = useQueryClient()
  const [mode, setMode] = useState<'due' | 'date'>('due')
  const [date, setDate] = useState(todayISO())

  // výchozí volby při příštím otevření
  const setOpen = (o: boolean) => {
    if (!o) {
      setMode('due')
      setDate(todayISO())
    }
    onOpenChange(o)
  }

  const preview = useQuery({
    queryKey: [...keys.account(slug), 'mark-paid', target],
    queryFn: ({ signal }) => markPaid(slug, target, { dry_run: true }, signal),
    enabled: open,
    gcTime: 0,
    staleTime: 0,
  })

  const run = useMutation({
    mutationFn: () => markPaid(slug, target, mode === 'date' ? { paid_on: date } : {}),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: keys.account(slug) })
      setOpen(false)
      toast.success(`Označeno jako uhrazené: ${countLabel(target.kind, res.count)}`)
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const count = preview.data?.count ?? 0
  const dateValid = mode === 'due' || parseISODate(date) !== null

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={setOpen}
      title="Označit jako uhrazené"
      description={
        filtered
          ? 'Všechny neuhrazené doklady podle aktuálního filtru.'
          : 'Všechny neuhrazené doklady — pro jen některé nejdřív nastavte filtr (např. období).'
      }
      footer={
        <>
          <Button onClick={() => run.mutate()} disabled={run.isPending || !preview.data || count === 0 || !dateValid}>
            {run.isPending && <Spinner data-icon="inline-start" />}
            {preview.data && count > 0 ? `Označit ${countLabel(target.kind, count)}` : 'Označit'}
          </Button>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <div className="rounded-lg border bg-muted/40 p-3 text-sm">
          {preview.isPending ? (
            <Skeleton className="h-5 w-48" />
          ) : preview.isError ? (
            <span className="text-destructive">{errorMessage(preview.error)}</span>
          ) : count === 0 ? (
            <span className="text-muted-foreground">Žádný neuhrazený doklad — není co označit.</span>
          ) : (
            <div className="flex flex-col gap-0.5">
              <span className="font-medium">{countLabel(target.kind, count)} k úhradě</span>
              {preview.data.sums.map((s) => (
                <span key={s.currency} className="tabular-nums text-muted-foreground">
                  {formatMoney(s.sum_remaining, s.currency)}
                </span>
              ))}
            </div>
          )}
        </div>

        <div className="flex flex-col gap-3">
          <Label>Datum úhrady</Label>
          <RadioGroup value={mode} onValueChange={(v) => setMode(v as 'due' | 'date')}>
            <Label className="flex items-start gap-3 font-normal">
              <RadioGroupItem value="due" className="mt-0.5" />
              <span className="flex flex-col gap-0.5">
                Ke dni splatnosti
                <span className="text-xs text-muted-foreground">U dokladů splatných v budoucnu dnešní datum.</span>
              </span>
            </Label>
            <Label className="flex items-center gap-3 font-normal">
              <RadioGroupItem value="date" />
              Jedno datum pro všechny
            </Label>
          </RadioGroup>
          {mode === 'date' && (
            <Input
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              aria-label="Datum úhrady"
              className="max-w-48"
            />
          )}
        </div>
        {target.kind === 'invoices' && (
          <p className="text-xs text-muted-foreground">Odběratelům se neposílá poděkování za úhradu.</p>
        )}
      </div>
    </ResponsiveDialog>
  )
}
