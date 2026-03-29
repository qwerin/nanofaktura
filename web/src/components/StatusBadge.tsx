import { Badge } from '@/components/ui/badge'
import type { InvoiceStatus } from '../api/client'

const LABELS: Record<InvoiceStatus, string> = {
  open:    'Neuhrazená',
  overdue: 'Po splatnosti',
  paid:    'Uhrazená',
}

const VARIANTS: Record<InvoiceStatus, string> = {
  open:    'bg-blue-100 text-blue-700 hover:bg-blue-100',
  overdue: 'bg-orange-100 text-orange-700 hover:bg-orange-100',
  paid:    'bg-green-100 text-green-700 hover:bg-green-100',
}

export function StatusBadge({ status }: { status: InvoiceStatus }) {
  return (
    <Badge className={VARIANTS[status] ?? 'bg-slate-100 text-slate-500'}>
      {LABELS[status] ?? status}
    </Badge>
  )
}
