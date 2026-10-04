import { BanIcon, CheckIcon, CircleAlertIcon, CircleDashedIcon, CircleSlashIcon, PencilLineIcon, SendIcon, type LucideIcon } from 'lucide-react'
import type { InvoiceStatus } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { statusLabels } from './status'

const styles: Record<InvoiceStatus, { className: string; icon: LucideIcon }> = {
  draft: { className: 'border border-dashed border-muted-foreground/40 bg-transparent text-muted-foreground', icon: PencilLineIcon },
  open: { className: 'bg-muted text-muted-foreground', icon: CircleDashedIcon },
  sent: { className: 'bg-info/12 text-info dark:bg-info/20', icon: SendIcon },
  overdue: { className: 'bg-destructive/10 text-destructive dark:bg-destructive/20', icon: CircleAlertIcon },
  paid: { className: 'bg-success/12 text-success dark:bg-success/20', icon: CheckIcon },
  cancelled: { className: 'bg-muted text-muted-foreground line-through decoration-muted-foreground/50', icon: BanIcon },
  uncollectible: { className: 'bg-warning/15 text-warning-foreground dark:text-warning', icon: CircleSlashIcon },
}

/** Štítek stavu faktury (ikona + text, nikdy jen barva). */
export function StatusBadge({ status, className }: { status: InvoiceStatus; className?: string }) {
  const s = styles[status]
  const Icon = s.icon
  return (
    <Badge variant="secondary" className={cn('gap-1', s.className, className)}>
      <Icon data-icon="inline-start" aria-hidden="true" />
      {statusLabels[status]}
    </Badge>
  )
}
