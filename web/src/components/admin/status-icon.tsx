import { CircleCheckIcon, CircleXIcon, InfoIcon, TriangleAlertIcon } from 'lucide-react'
import { statusLabels, type DiagStatus } from '@/lib/mail-diag'
import { cn } from '@/lib/utils'

const icons = {
  ok: { Icon: CircleCheckIcon, className: 'text-success' },
  info: { Icon: InfoIcon, className: 'text-muted-foreground' },
  warning: { Icon: TriangleAlertIcon, className: 'text-warning' },
  error: { Icon: CircleXIcon, className: 'text-destructive' },
} satisfies Record<DiagStatus, unknown>

/** Ikona stavu kontroly (ok / info / varování / chyba) s popiskem pro čtečky. */
export function StatusIcon({ status, className }: { status: DiagStatus; className?: string }) {
  const { Icon, className: color } = icons[status]
  return <Icon role="img" aria-label={statusLabels[status]} className={cn('size-5 shrink-0', color, className)} />
}
