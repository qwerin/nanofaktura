import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowDownLeftIcon,
  ArrowUpRightIcon,
  BanIcon,
  CheckIcon,
  CircleDashedIcon,
  FileTextIcon,
  ReceiptIcon,
  SparklesIcon,
  type LucideIcon,
} from 'lucide-react'
import { expenseQueries } from '@/api/queries/expenses'
import { invoiceQueries } from '@/api/queries/invoices'
import type { BankTransaction, BankTransactionState } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { amountToneClass, formatSignedAmount, sortReasons, stateLabels, type ReasonTone } from './format'

/** Částka se znaménkem, příchozí zeleně. */
export function SignedAmount({ amount, currency, className }: { amount: number; currency: string; className?: string }) {
  return (
    <span className={cn('font-semibold whitespace-nowrap tabular-nums', amountToneClass(amount), className)}>
      {formatSignedAmount(amount, currency)}
    </span>
  )
}

/** Kulatá ikona směru pohybu (↙ příchozí / ↗ odchozí) — nikdy jen barva. */
export function DirectionIcon({ amount, className }: { amount: number; className?: string }) {
  const incoming = amount >= 0
  const Icon = incoming ? ArrowDownLeftIcon : ArrowUpRightIcon
  return (
    <span
      className={cn(
        'flex size-9 shrink-0 items-center justify-center rounded-full',
        incoming ? 'bg-success/12 text-success dark:bg-success/20' : 'bg-muted text-muted-foreground',
        className,
      )}
      aria-label={incoming ? 'Příchozí platba' : 'Odchozí platba'}
      role="img"
    >
      <Icon className="size-4" />
    </span>
  )
}

const toneClass: Record<ReasonTone, string> = {
  strong: 'bg-success/12 text-success dark:bg-success/20',
  partial: 'bg-warning/15 text-warning-foreground dark:text-warning',
  weak: 'bg-muted text-muted-foreground',
}

/** Čipy důvodů návrhu („VS sedí“, „Částka sedí“, „Částečná úhrada“…). */
export function ReasonChips({ reasons, className }: { reasons: readonly string[]; className?: string }) {
  if (reasons.length === 0) return null
  return (
    <span className={cn('flex flex-wrap gap-1', className)}>
      {sortReasons(reasons).map((r) => (
        <Badge key={r.label} variant="secondary" className={cn('gap-1 font-normal', toneClass[r.tone])}>
          {r.tone === 'strong' && <CheckIcon data-icon="inline-start" aria-hidden="true" />}
          {r.label}
        </Badge>
      ))}
    </span>
  )
}

const stateStyles: Record<BankTransactionState, { className: string; icon: LucideIcon }> = {
  unmatched: { className: 'bg-muted text-muted-foreground', icon: CircleDashedIcon },
  suggested: { className: 'bg-info/12 text-info dark:bg-info/20', icon: SparklesIcon },
  matched: { className: 'bg-success/12 text-success dark:bg-success/20', icon: CheckIcon },
  ignored: { className: 'bg-muted text-muted-foreground', icon: BanIcon },
}

export function TransactionStateBadge({ transaction: t, className }: { transaction: BankTransaction; className?: string }) {
  const s = stateStyles[t.state]
  const Icon = s.icon
  const label = t.state === 'matched' && t.auto_matched ? 'Spárováno automaticky' : stateLabels[t.state]
  return (
    <Badge variant="secondary" className={cn('gap-1', s.className, className)}>
      <Icon data-icon="inline-start" aria-hidden="true" />
      {label}
    </Badge>
  )
}

/**
 * Odkaz na spárovaný doklad. Transakce nese jen ID dokladu → číslo a jméno se dočtou
 * z detailu (cache sdílená s detailem dokladu); do té doby „Faktura“/„Náklad“.
 */
export function MatchedDocumentLink({ slug, transaction: t, className }: { slug: string; transaction: BankTransaction; className?: string }) {
  const invoiceId = t.matched_invoice_id
  const expenseId = t.matched_expense_id
  const invoice = useQuery({ ...invoiceQueries.detail(slug, invoiceId ?? 0), enabled: Boolean(invoiceId), meta: { silent: true } })
  const expense = useQuery({ ...expenseQueries.detail(slug, expenseId ?? 0), enabled: Boolean(expenseId), meta: { silent: true } })
  const linkClass = cn(
    'inline-flex min-h-9 max-w-full items-center gap-1.5 rounded-md text-sm font-medium text-primary hover:underline md:min-h-0',
    className,
  )

  if (invoiceId) {
    const inv = invoice.data
    return (
      <Link to="/a/$slug/invoices/$invoiceId" params={{ slug, invoiceId }} className={linkClass}>
        <FileTextIcon className="size-4 shrink-0" />
        <span className="truncate">
          {inv ? `${inv.document_type === 'proforma' ? 'Záloha' : 'Faktura'} ${inv.number}` : 'Faktura'}
          {inv?.client_name && <span className="font-normal text-muted-foreground"> · {inv.client_name}</span>}
        </span>
      </Link>
    )
  }
  if (expenseId) {
    const exp = expense.data
    return (
      <Link to="/a/$slug/expenses/$expenseId" params={{ slug, expenseId }} className={linkClass}>
        <ReceiptIcon className="size-4 shrink-0" />
        <span className="truncate">
          {exp ? `Náklad ${exp.number}` : 'Náklad'}
          {exp?.supplier_name && <span className="font-normal text-muted-foreground"> · {exp.supplier_name}</span>}
        </span>
      </Link>
    )
  }
  return null
}
