import type { ReactNode } from 'react'
import { LogoMark } from '@/components/logo'
import { cn } from '@/lib/utils'

/** Rozvržení veřejných stránek (přihlášení, registrace). Mobil: na celou šířku, desktop: karta uprostřed. */
export function AuthLayout({
  title,
  description,
  children,
  footer,
  className,
}: {
  title?: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  className?: string
}) {
  return (
    <div className="relative flex min-h-dvh flex-col bg-background pt-safe pb-safe px-safe">
      {/* jemný dekorativní přechod v pozadí */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(60%_100%_at_50%_0%,color-mix(in_oklch,var(--primary)_12%,transparent),transparent)]"
      />
      <main className="relative flex flex-1 flex-col px-5 py-10 sm:items-center sm:justify-center sm:px-6">
        <div className={cn('flex w-full flex-col gap-8 sm:max-w-sm', className)}>
          <div className="flex flex-col gap-5">
            <div className="flex items-center gap-2.5">
              <LogoMark className="size-9" />
              <span className="text-lg font-semibold tracking-tight">NanoFaktura</span>
            </div>
            {(title || description) && (
              <div className="flex flex-col gap-1.5">
                {title && <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>}
                {description && <p className="text-sm text-muted-foreground">{description}</p>}
              </div>
            )}
          </div>
          {children}
          {footer && <div className="text-center text-sm text-muted-foreground">{footer}</div>}
        </div>
      </main>
    </div>
  )
}
