import { cn } from '@/lib/utils'

export interface ChipItem {
  token: string
  label: string
  /** Ukázková hodnota (např. „září“). */
  example?: string
}

/**
 * Řada klikacích placeholderů (vkládání přes `usePlaceholderInsert`).
 * `onMouseDown` brání ztrátě fokusu a výběru v editovaném poli.
 */
export function PlaceholderChips({
  items,
  onInsert,
  disabled,
  className,
}: {
  items: readonly ChipItem[]
  onInsert: (token: string) => void
  disabled?: boolean
  className?: string
}) {
  return (
    <div className={cn('flex flex-wrap gap-1.5', className)}>
      {items.map((p) => (
        <button
          key={p.token}
          type="button"
          disabled={disabled}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => onInsert(p.token)}
          title={`Vložit ${p.token} (${p.label})`}
          className={cn(
            'inline-flex min-h-9 items-center gap-1.5 rounded-md border bg-background px-2 text-left text-xs transition-colors md:min-h-7',
            'hover:border-primary/40 hover:bg-accent focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
            'disabled:pointer-events-none disabled:opacity-50 dark:bg-input/30',
          )}
        >
          <code className="font-mono text-[0.7rem] font-medium text-primary">{p.token}</code>
          {p.example !== undefined ? (
            <span className="text-muted-foreground">→ {p.example || '—'}</span>
          ) : (
            <span className="text-muted-foreground">{p.label}</span>
          )}
        </button>
      ))}
    </div>
  )
}
