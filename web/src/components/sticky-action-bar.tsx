import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface StickyActionBarProps {
  children: ReactNode
  /** Doplňující obsah vlevo (např. „Neuložené změny“ nebo součet faktury). */
  start?: ReactNode
  className?: string
}

/**
 * Spodní lišta s primární akcí formuláře (Uložit / Vystavit).
 *
 * - mobil: fixně nad spodním tab barem (respektuje safe area); při otevřené klávesnici
 *   se tab bar skryje a lišta sedí přímo nad klávesnicí. Tlačítka se roztáhnou na šířku.
 * - desktop: `sticky` u spodního okraje obsahu.
 *
 * Vložte ji jako poslední prvek formuláře — sama si za sebou nechá místo, aby nepřekrývala obsah.
 */
export function StickyActionBar({ children, start, className }: StickyActionBarProps) {
  return (
    <>
      {/* Rezerva pod obsahem, aby fixní lišta na mobilu nepřekryla poslední pole. */}
      <div aria-hidden="true" className="h-20 md:hidden" />
      <div
        data-slot="sticky-action-bar"
        className={cn(
          'z-30 border-t bg-background/85 backdrop-blur-md supports-backdrop-filter:bg-background/70',
          // mobil
          'fixed inset-x-0 bottom-[calc(var(--spacing-tabbar)+env(safe-area-inset-bottom))] px-4 py-3',
          'keyboard-open:bottom-(--keyboard-inset,0px)',
          // desktop
          'md:sticky md:bottom-0 md:-mx-8 md:mt-8 md:px-8',
          className,
        )}
      >
        <div className="mx-auto flex w-full max-w-6xl items-center gap-3">
          {start && <div className="hidden min-w-0 flex-1 text-sm text-muted-foreground md:block">{start}</div>}
          <div className="flex flex-1 items-center gap-2 *:flex-1 md:ml-auto md:flex-none md:*:flex-none">
            {children}
          </div>
        </div>
      </div>
    </>
  )
}
