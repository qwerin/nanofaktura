import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  BanknoteIcon,
  LockIcon,
  LockOpenIcon,
  PackageIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { expenseQueries, useDeleteExpense, useDeleteExpensePayment, useExpenseAction } from '@/api/queries/expenses'
import type { Expense, ExpensePayment } from '@/api/types'
import { Attachments } from '@/components/attachments/attachments'
import { HistorySection } from '@/components/events/timeline'
import { ExpenseWarnings } from '@/components/bank/expense-warnings'
import { ConfirmDialog } from '@/components/expense/confirm-dialog'
import { ExpenseStatusBadge } from '@/components/expense/expense-status-badge'
import { paymentMethodLabel } from '@/components/expense/format'
import { ExpensePaymentDialog } from '@/components/expense/payment-dialog'
import { useCanEditDocuments } from '@/components/expense/permissions'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ListSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { formatDate, formatDateTime, formatDueRelative } from '@/lib/date'
import { formatMoney, formatVatRate } from '@/lib/money'
import { parseId } from '@/lib/params'
import { formatQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/expenses/$expenseId/')({
  params: {
    parse: ({ expenseId }) => ({ expenseId: parseId(expenseId) }),
    stringify: ({ expenseId }) => ({ expenseId: String(expenseId) }),
  },
  loader: ({ context, params }) => context.queryClient.prefetchQuery(expenseQueries.detail(params.slug, params.expenseId)),
  head: () => ({ meta: [{ title: 'Náklad · NanoFaktura' }] }),
  component: ExpenseDetailPage,
})

function ExpenseDetailPage() {
  const { slug, expenseId } = Route.useParams()
  const navigate = useNavigate()
  const canEdit = useCanEditDocuments()
  const expense = useQuery(expenseQueries.detail(slug, expenseId))
  const account = useQuery(accountQueries.detail(slug))
  const action = useExpenseAction(slug, expenseId)
  const remove = useDeleteExpense(slug)
  const [paymentOpen, setPaymentOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const back = { to: '/a/$slug/expenses', params: { slug } } as const

  if (expense.isError) return <PageError error={expense.error} reset={() => void expense.refetch()} />
  if (expense.isPending) {
    return (
      <>
        <PageHeader title="Náklad" back={back} />
        <PageBody>
          <ListSkeleton rows={4} />
        </PageBody>
      </>
    )
  }

  const e = expense.data
  const locked = Boolean(e.locked_at)
  const actions: PageAction[] = canEdit
    ? [
        {
          label: 'Upravit',
          icon: PencilIcon,
          primary: true,
          disabled: locked,
          render: locked ? undefined : <Link to="/a/$slug/expenses/$expenseId/edit" params={{ slug, expenseId }} />,
        },
        ...(e.remaining_amount !== 0
          ? [{ label: 'Přidat úhradu', icon: BanknoteIcon, onClick: () => setPaymentOpen(true) } satisfies PageAction]
          : []),
        {
          label: locked ? 'Odemknout' : 'Zamknout',
          icon: locked ? LockOpenIcon : LockIcon,
          disabled: action.isPending,
          onClick: () =>
            action.mutate(locked ? 'unlock' : 'lock', {
              onSuccess: () => toast.success(locked ? 'Náklad odemčen' : 'Náklad zamčen'),
            }),
        },
        { label: 'Smazat', icon: Trash2Icon, variant: 'destructive', onClick: () => setDeleteOpen(true) },
      ]
    : []

  return (
    <>
      <PageHeader title={e.number} description={e.supplier_name} back={back} actions={actions} />
      <PageBody className="grid gap-4 md:gap-6 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start">
        <div className="flex min-w-0 flex-col gap-4 md:gap-6">
          <ExpenseWarnings warnings={e.warnings} />
          <HeroCard expense={e} />
          <Card title="Položky">
            <LinesTable expense={e} />
          </Card>
          <Card>
            <Attachments ownerType="expense" ownerId={e.id} canEdit={canEdit} emptyText="Doklad zatím nemá žádnou přílohu." />
          </Card>
        </div>

        <aside className="flex min-w-0 flex-col gap-4 md:gap-6">
          <PaymentsCard expense={e} slug={slug} canEdit={canEdit} onAdd={() => setPaymentOpen(true)} />
          <Card title="Údaje">
            <InfoList expense={e} vatPayer={account.data?.vat_mode === 'vat_payer'} />
          </Card>
          <HistorySection subjectType="expense" subjectId={e.id} />
        </aside>
      </PageBody>

      {canEdit && <ExpensePaymentDialog expense={e} slug={slug} open={paymentOpen} onOpenChange={setPaymentOpen} />}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Smazat náklad?"
        description={
          locked
            ? 'Náklad je zamčený — nejdřív ho odemkněte.'
            : e.payments.length > 0
              ? 'Náklad má zaznamenané úhrady — nejdřív je smažte.'
              : `Náklad ${e.number} bude trvale odstraněn. Akci nelze vrátit.`
        }
        confirmLabel="Smazat náklad"
        destructive
        pending={remove.isPending}
        onConfirm={() =>
          remove.mutate(e.id, {
            onSuccess: () => {
              toast.success('Náklad smazán')
              setDeleteOpen(false)
              void navigate({ ...back, replace: true })
            },
          })
        }
      />
    </>
  )
}

function Card({ title, action, children, className }: { title?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn('rounded-xl border bg-card p-4 text-card-foreground md:p-5', className)}>
      {title && (
        <div className="mb-3 flex items-center gap-2">
          <h2 className="flex-1 text-base font-semibold tracking-tight">{title}</h2>
          {action}
        </div>
      )}
      {children}
    </section>
  )
}

function HeroCard({ expense: e }: { expense: Expense }) {
  const { slug } = Route.useParams()
  const address = [e.supplier_street, [e.supplier_zip, e.supplier_city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
  return (
    <Card>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <ExpenseStatusBadge status={e.status} />
            {e.locked_at && (
              <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                <LockIcon className="size-3.5" /> Zamčeno
              </span>
            )}
            {!e.tax_deductible && <span className="text-xs text-muted-foreground">· daňově neuznatelný</span>}
          </div>
          <p className="mt-1 text-lg font-semibold tracking-tight">
            {e.subject_id ? (
              <Link to="/a/$slug/subjects/$subjectId" params={{ slug, subjectId: e.subject_id }} className="hover:underline">
                {e.supplier_name}
              </Link>
            ) : (
              e.supplier_name
            )}
          </p>
          <p className="text-sm text-muted-foreground">
            {[e.supplier_registration_no && `IČO ${e.supplier_registration_no}`, e.supplier_vat_no && `DIČ ${e.supplier_vat_no}`].filter(Boolean).join(' · ')}
          </p>
          {address && <p className="text-sm text-muted-foreground">{address}</p>}
        </div>
        <div className="flex flex-col gap-0.5 border-t pt-3 sm:items-end sm:border-t-0 sm:pt-0 sm:text-right">
          <span className="text-sm text-muted-foreground">Celkem</span>
          <span className="text-2xl font-semibold tracking-tight tabular-nums">{formatMoney(e.total, e.currency)}</span>
          {e.status !== 'paid' && e.paid_amount !== 0 && (
            <span className="text-sm text-muted-foreground tabular-nums">zbývá {formatMoney(e.remaining_amount, e.currency)}</span>
          )}
          {e.status === 'paid' && e.paid_on && <span className="text-sm text-success">uhrazeno {formatDate(e.paid_on)}</span>}
        </div>
      </div>
      <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 border-t pt-4 text-sm sm:grid-cols-4">
        <Fact label="Vystaveno" value={formatDate(e.issued_on)} />
        <Fact label="DUZP" value={formatDate(e.taxable_fulfillment_due) || '—'} />
        <Fact
          label="Splatnost"
          value={
            e.due_on ? (
              <span className="flex flex-col">
                {formatDate(e.due_on)}
                {e.status !== 'paid' && (
                  <span className={cn('text-xs', e.status === 'overdue' ? 'text-destructive' : 'text-muted-foreground')}>
                    {formatDueRelative(e.due_on)}
                  </span>
                )}
              </span>
            ) : (
              '—'
            )
          }
        />
        <Fact label="Č. dokladu" value={e.original_number || '—'} />
      </dl>
    </Card>
  )
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate font-medium">{value}</dd>
    </div>
  )
}

function LinesTable({ expense: e }: { expense: Expense }) {
  const showVat = e.vat_total !== 0 || e.vat_recap.some((r) => r.vat_rate_bps !== 0)
  return (
    <div className="flex flex-col gap-4">
      {/* Mobil: karty */}
      <ul className="flex flex-col divide-y md:hidden">
        {e.lines.map((l) => (
          <li key={l.id} className="flex flex-col gap-1 py-3 first:pt-0">
            <div className="flex items-start gap-3">
              <span className="min-w-0 flex-1 font-medium">
                {l.name}
                {l.price_item_id && <PriceItemLink id={l.price_item_id} />}
              </span>
              <span className="shrink-0 font-medium tabular-nums">{formatMoney(l.total, e.currency)}</span>
            </div>
            <span className="text-sm text-muted-foreground tabular-nums">
              {formatQuantity(l.quantity)} {l.unit_name} × {formatMoney(l.unit_price, e.currency)}
              {showVat && ` · DPH ${formatVatRate(l.vat_rate_bps)}`}
            </span>
          </li>
        ))}
      </ul>

      {/* Desktop: tabulka */}
      <table className="hidden w-full text-sm md:table">
        <thead>
          <tr className="border-b text-xs text-muted-foreground">
            <th className="py-2 pr-2 text-left font-medium">Položka</th>
            <th className="px-2 py-2 text-right font-medium">Množství</th>
            <th className="px-2 py-2 text-right font-medium">Cena/jedn. {e.prices_include_vat ? 's DPH' : 'bez DPH'}</th>
            {showVat && <th className="px-2 py-2 text-right font-medium">DPH</th>}
            <th className="py-2 pl-2 text-right font-medium">Celkem s DPH</th>
          </tr>
        </thead>
        <tbody>
          {e.lines.map((l) => (
            <tr key={l.id} className="border-b last:border-b-0">
              <td className="py-2.5 pr-2">
                {l.name}
                {l.price_item_id && <PriceItemLink id={l.price_item_id} />}
              </td>
              <td className="px-2 py-2.5 text-right tabular-nums whitespace-nowrap">
                {formatQuantity(l.quantity)} {l.unit_name}
              </td>
              <td className="px-2 py-2.5 text-right tabular-nums whitespace-nowrap">{formatMoney(l.unit_price, e.currency)}</td>
              {showVat && <td className="px-2 py-2.5 text-right tabular-nums">{formatVatRate(l.vat_rate_bps)}</td>}
              <td className="py-2.5 pl-2 text-right font-medium tabular-nums whitespace-nowrap">{formatMoney(l.total, e.currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <div className="flex justify-end border-t pt-4">
        <dl className="flex w-full flex-col gap-1.5 text-sm sm:max-w-xs">
          {showVat && (
            <>
              <Row label="Základ" value={formatMoney(e.subtotal, e.currency)} />
              {e.vat_recap.map((r) => (
                <Row key={r.vat_rate_bps} label={`DPH ${formatVatRate(r.vat_rate_bps)} (ze ${formatMoney(r.base, e.currency)})`} value={formatMoney(r.vat, e.currency)} />
              ))}
            </>
          )}
          {e.rounding !== 0 && <Row label="Zaokrouhlení" value={formatMoney(e.rounding, e.currency)} />}
          <div className="mt-1 flex items-baseline justify-between gap-4 border-t pt-2">
            <dt className="font-medium">Celkem</dt>
            <dd className="text-lg font-semibold tabular-nums">{formatMoney(e.total, e.currency)}</dd>
          </div>
          {e.currency !== 'CZK' && e.exchange_rate && e.exchange_rate !== '1' && (
            <p className="text-right text-xs text-muted-foreground">kurz {e.exchange_rate.replace('.', ',')}</p>
          )}
        </dl>
      </div>
    </div>
  )
}

function PriceItemLink({ id }: { id: number }) {
  const { slug } = Route.useParams()
  return (
    <Link
      to="/a/$slug/price-items/$priceItemId"
      params={{ slug, priceItemId: id }}
      className="ml-2 inline-flex translate-y-0.5 items-center gap-1 text-xs text-muted-foreground hover:text-primary"
      title="Položka ceníku"
    >
      <PackageIcon className="size-3.5" />
      ceník
    </Link>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="tabular-nums">{value}</dd>
    </div>
  )
}

function PaymentsCard({ expense: e, slug, canEdit, onAdd }: { expense: Expense; slug: string; canEdit: boolean; onAdd: () => void }) {
  const remove = useDeleteExpensePayment(slug, e.id)
  const [deleting, setDeleting] = useState<ExpensePayment | null>(null)
  const paidPct = e.total !== 0 ? Math.min(100, Math.max(0, Math.round((e.paid_amount / e.total) * 100))) : 0

  return (
    <Card
      title="Úhrady"
      action={
        canEdit &&
        e.remaining_amount !== 0 && (
          <Button variant="outline" size="sm" className="h-9 md:h-7" onClick={onAdd}>
            <PlusIcon data-icon="inline-start" />
            Přidat
          </Button>
        )
      }
    >
      <div className="mb-3 flex flex-col gap-2">
        <div className="flex items-baseline justify-between text-sm">
          <span className="text-muted-foreground">Uhrazeno</span>
          <span className="font-medium tabular-nums">
            {formatMoney(e.paid_amount, e.currency)} <span className="text-muted-foreground">z {formatMoney(e.total, e.currency)}</span>
          </span>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-muted">
          <div className={cn('h-full rounded-full', e.status === 'paid' ? 'bg-success' : 'bg-primary')} style={{ width: `${paidPct}%` }} />
        </div>
      </div>
      {e.payments.length === 0 ? (
        <p className="text-sm text-muted-foreground">Zatím bez úhrady.</p>
      ) : (
        <ul className="flex flex-col divide-y">
          {e.payments.map((p) => (
            <li key={p.id} className="flex items-center gap-3 py-2">
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="text-sm font-medium tabular-nums">{formatMoney(p.amount, e.currency)}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {formatDate(p.paid_on)}
                  {p.note && ` · ${p.note}`}
                </span>
              </div>
              {canEdit && (
                <Button variant="ghost" size="icon" className="size-9 text-muted-foreground hover:text-destructive md:size-7" aria-label="Smazat úhradu" onClick={() => setDeleting(p)}>
                  <Trash2Icon />
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="Smazat úhradu?"
        description={deleting && `Úhrada ${formatMoney(deleting.amount, e.currency)} z ${formatDate(deleting.paid_on)} bude odstraněna.`}
        confirmLabel="Smazat úhradu"
        destructive
        pending={remove.isPending}
        onConfirm={() =>
          deleting &&
          remove.mutate(deleting.id, {
            onSuccess: () => {
              toast.success('Úhrada smazána')
              setDeleting(null)
            },
          })
        }
      />
    </Card>
  )
}

function InfoList({ expense: e, vatPayer }: { expense: Expense; vatPayer: boolean }) {
  const rows: [string, ReactNode][] = [
    ['Interní číslo', e.number],
    ['Variabilní symbol', e.variable_symbol || '—'],
    ['Kategorie', e.category || '—'],
    ['Úhrada', paymentMethodLabel(e.payment_method)],
    ['Měna', e.currency + (e.currency !== 'CZK' && e.exchange_rate !== '1' ? ` (kurz ${e.exchange_rate.replace('.', ',')})` : '')],
    ['Daňově uznatelný', e.tax_deductible ? 'Ano' : 'Ne'],
    ...(vatPayer ? [['Odpočet DPH', e.vat_deductible ? 'Ano' : 'Ne'] as [string, ReactNode]] : []),
    ['Ceny', e.prices_include_vat ? 'včetně DPH' : 'bez DPH'],
  ]
  if (e.reverse_charge) {
    rows.push(['Přenesená daňová povinnost', `Daň přiznáváte vy · ${e.supply_type === 'goods' ? 'zboží' : 'služba'}`])
  }
  if (e.supplier_iban || e.supplier_bank_account) rows.push(['Účet dodavatele', e.supplier_bank_account || e.supplier_iban])
  return (
    <div className="flex flex-col gap-4">
      {e.description && <p className="text-sm">{e.description}</p>}
      <dl className="flex flex-col gap-2 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-4">
            <dt className="shrink-0 text-muted-foreground">{k}</dt>
            <dd className="min-w-0 truncate text-right">{v}</dd>
          </div>
        ))}
      </dl>
      {e.private_note && (
        <div className="rounded-lg bg-muted/50 p-3 text-sm">
          <p className="mb-1 text-xs font-medium text-muted-foreground">Interní poznámka</p>
          <p className="whitespace-pre-wrap">{e.private_note}</p>
        </div>
      )}
      <p className="text-xs text-muted-foreground">
        Vytvořeno {formatDateTime(e.created_at)}
        {e.updated_at !== e.created_at && ` · upraveno ${formatDateTime(e.updated_at)}`}
        {e.locked_at && ` · zamčeno ${formatDateTime(e.locked_at)}`}
      </p>
    </div>
  )
}
