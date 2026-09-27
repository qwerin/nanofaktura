import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronRightIcon, FilePlusIcon, FileTextIcon } from 'lucide-react'
import { subjectQueries } from '@/api/queries/subjects'
import type { ResponseBody } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'

type InvoiceRow = NonNullable<ResponseBody<'/api/accounts/{slug}/invoices', 'get'>['items']>[number]

const statusLabels: Record<InvoiceRow['status'], string> = {
  open: 'Vystavená',
  sent: 'Odeslaná',
  overdue: 'Po splatnosti',
  paid: 'Uhrazená',
  cancelled: 'Stornovaná',
  uncollectible: 'Nedobytná',
}

const statusClass: Record<InvoiceRow['status'], string> = {
  open: 'bg-info/10 text-info',
  sent: 'bg-info/10 text-info',
  overdue: 'bg-destructive/10 text-destructive',
  paid: 'bg-success/10 text-success',
  cancelled: 'bg-muted text-muted-foreground line-through',
  uncollectible: 'bg-muted text-muted-foreground',
}

const docTypeLabels: Record<InvoiceRow['document_type'], string | null> = {
  invoice: null,
  proforma: 'Záloha',
  correction: 'Opravný doklad',
}

/** Doklady kontaktu (jen čtení) — posledních 20, odkaz na detail faktury. */
export function SubjectInvoices({ slug, subjectId, canCreate = true }: { slug: string; subjectId: number; canCreate?: boolean }) {
  const invoices = useQuery(subjectQueries.invoices(slug, subjectId))
  const newInvoice = canCreate && (
    <Button
      variant="outline"
      nativeButton={false}
      render={<Link to="/a/$slug/invoices/new" params={{ slug }} search={{ subject_id: subjectId }} />}
    >
      <FilePlusIcon data-icon="inline-start" />
      Nová faktura
    </Button>
  )

  return (
    <section aria-labelledby="subject-invoices" className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <h2 id="subject-invoices" className="text-base font-semibold tracking-tight">
          Faktury
          {invoices.data && invoices.data.total > 0 && (
            <span className="ml-2 text-sm font-normal text-muted-foreground">{invoices.data.total}</span>
          )}
        </h2>
        {invoices.data && invoices.data.total > 0 && <div className="max-md:hidden">{newInvoice}</div>}
      </div>

      {invoices.isPending ? (
        <div className="flex flex-col gap-2" aria-hidden="true">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-16 w-full rounded-xl" />
          ))}
        </div>
      ) : invoices.isError ? (
        <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">
          Faktury se nepodařilo načíst.{' '}
          <button type="button" className="font-medium text-foreground underline" onClick={() => void invoices.refetch()}>
            Zkusit znovu
          </button>
        </p>
      ) : !invoices.data.items?.length ? (
        <EmptyState
          icon={FileTextIcon}
          title="Zatím žádné faktury"
          description="Vystavte tomuto kontaktu první fakturu."
          action={newInvoice}
          className="py-8"
        />
      ) : (
        <ul className="overflow-hidden rounded-xl border bg-card">
          {invoices.data.items.map((inv) => (
            <li key={inv.id} className="border-b last:border-b-0">
              <Link
                to="/a/$slug/invoices/$invoiceId"
                params={{ slug, invoiceId: inv.id }}
                className="flex min-h-16 items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/50 active:bg-muted"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="flex items-center gap-2">
                    <span className="truncate font-medium tabular-nums">{inv.number}</span>
                    {docTypeLabels[inv.document_type] && (
                      <span className="text-xs text-muted-foreground">{docTypeLabels[inv.document_type]}</span>
                    )}
                  </span>
                  <span className="text-sm text-muted-foreground">
                    {formatDate(inv.issued_on)} · splatnost {formatDate(inv.due_on)}
                  </span>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <span className="font-medium tabular-nums">{formatMoney(inv.total, inv.currency)}</span>
                  <Badge className={cn('border-transparent', statusClass[inv.status])}>{statusLabels[inv.status]}</Badge>
                </div>
                <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
              </Link>
            </li>
          ))}
        </ul>
      )}
      {invoices.data && invoices.data.total > 0 && <div className="md:hidden [&>*]:w-full">{newInvoice}</div>}
    </section>
  )
}
