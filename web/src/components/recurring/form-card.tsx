import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/** Karta sekce formuláře / detailu (stejný vzhled jako ve formuláři faktury). */
export function FormCard({
  title,
  description,
  children,
  className,
  action,
}: {
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  className?: string
  action?: ReactNode
}) {
  return (
    <section className={cn('flex flex-col gap-4 rounded-xl border bg-card p-4 text-card-foreground md:p-5', className)}>
      <div className="flex min-h-7 items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 className="text-base font-semibold tracking-tight">{title}</h2>
          {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}
