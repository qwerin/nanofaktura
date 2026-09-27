import type { MemberRole } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { initials } from '@/components/layout/nav'
import { roleLabels } from '@/lib/roles'
import { cn } from '@/lib/utils'

const roleStyles: Record<MemberRole, string> = {
  owner: 'border-transparent bg-primary/10 text-primary',
  admin: 'border-transparent bg-info/10 text-info',
  accountant: 'border-transparent bg-warning/15 text-warning-foreground dark:text-warning',
  member: 'border-transparent bg-muted text-muted-foreground',
}

export function RoleBadge({ role, className }: { role: MemberRole; className?: string }) {
  return <Badge className={cn(roleStyles[role], className)}>{roleLabels[role]}</Badge>
}

/** Kulatý avatar s iniciálami (bez obrázků — uživatelé nemají fotky). */
export function MemberAvatar({ name, className }: { name: string; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex size-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-primary',
        className,
      )}
    >
      {initials(name)}
    </span>
  )
}
