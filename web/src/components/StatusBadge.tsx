import { Badge } from '@/components/ui/badge'
import type { InvoiceStatus } from '../api/client'

const LABELS: Record<InvoiceStatus, string> = {
  open:      'Otevřená',
  sent:      'Odeslaná',
  overdue:   'Po splatnosti',
  paid:      'Zaplacená',
  cancelled: 'Stornovaná',
}

const VARIANTS: Record<InvoiceStatus, string> = {
  open:      'bg-blue-100 text-blue-700 hover:bg-blue-100',
  sent:      'bg-indigo-100 text-indigo-700 hover:bg-indigo-100',
  overdue:   'bg-orange-100 text-orange-700 hover:bg-orange-100',
  paid:      'bg-green-100 text-green-700 hover:bg-green-100',
  cancelled: 'bg-slate-100 text-slate-500 hover:bg-slate-100',
}

export function StatusBadge({ status }: { status: InvoiceStatus }) {
  return (
    <Badge className={VARIANTS[status] ?? ''}>
      {LABELS[status] ?? status}
    </Badge>
  )
}
