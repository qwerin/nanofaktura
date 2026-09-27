import { LockIcon } from 'lucide-react'
import type { ExpenseStatus } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { expenseStatusLabels } from './format'

const styles: Record<ExpenseStatus, string> = {
  open: 'border-info/30 bg-info/10 text-info',
  overdue: 'border-destructive/30 bg-destructive/10 text-destructive',
  paid: 'border-success/30 bg-success/10 text-success',
}

export function ExpenseStatusBadge({ status, locked, className }: { status: ExpenseStatus; locked?: boolean; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1', className)}>
      <Badge variant="outline" className={styles[status]}>
        {expenseStatusLabels[status]}
      </Badge>
      {locked && (
        <LockIcon className="size-3.5 text-muted-foreground" aria-label="Zamčeno" />
      )}
    </span>
  )
}
