import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/** Přepínač s několika volbami (touch-friendly, role radiogroup). */
export function Segmented<V extends string>({
  value,
  onChange,
  options,
  label,
  disabled,
}: {
  value: V
  onChange: (v: V) => void
  options: readonly { value: V; label: ReactNode }[]
  label: string
  disabled?: boolean
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="grid gap-1 rounded-lg bg-muted p-1"
      style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))` }}
    >
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          disabled={disabled}
          onClick={() => onChange(o.value)}
          className={cn(
            'h-10 rounded-md px-2 text-sm font-medium text-muted-foreground transition-colors md:h-8',
            'focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none disabled:opacity-50',
            value === o.value ? 'bg-background text-foreground shadow-sm dark:bg-input/40' : 'hover:text-foreground',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}
