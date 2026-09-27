import { TriangleAlertIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/** Odznak „Nízký stav“ u skladových položek ceníku. */
export function StockBadge({ low, className }: { low: boolean; className?: string }) {
  if (!low) return null
  return (
    <Badge variant="outline" className={cn('border-warning/40 bg-warning/15 text-warning-foreground dark:text-warning', className)}>
      <TriangleAlertIcon data-icon="inline-start" />
      Nízký stav
    </Badge>
  )
}
