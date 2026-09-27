import type { MemberRole } from '@/api/types'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { roleLabels } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { roleDescriptions } from './permissions'

/** Výběr role jako karty s krátkým vysvětlením (pozvánka, změna role). */
export function RolePicker({
  value,
  onChange,
  roles,
  name,
  className,
  'aria-labelledby': labelledBy,
}: {
  value: MemberRole
  onChange: (role: MemberRole) => void
  roles: readonly MemberRole[]
  name: string
  className?: string
  'aria-labelledby'?: string
}) {
  return (
    <RadioGroup
      name={name}
      value={value}
      onValueChange={(v) => onChange(v as MemberRole)}
      aria-labelledby={labelledBy}
      className={cn('gap-2', className)}
    >
      {roles.map((role) => (
        <label
          key={role}
          className={cn(
            'flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors',
            'hover:bg-muted/50 has-[[data-checked]]:border-primary has-[[data-checked]]:bg-primary/5',
          )}
        >
          <RadioGroupItem value={role} className="mt-0.5" />
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="text-sm font-medium">{roleLabels[role]}</span>
            <span className="text-sm text-muted-foreground">{roleDescriptions[role]}</span>
          </span>
        </label>
      ))}
    </RadioGroup>
  )
}
