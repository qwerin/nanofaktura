import { Link } from '@tanstack/react-router'
import { ChevronLeftIcon, ChevronRightIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/** Přepínač sekcí přehledů (segmentované odkazy — funguje stejně na mobilu i desktopu). */
export function ReportsNav({ slug, className }: { slug: string; className?: string }) {
  const cls =
    'flex h-9 flex-1 items-center justify-center rounded-md px-4 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground data-[status=active]:bg-background data-[status=active]:text-foreground data-[status=active]:shadow-sm md:h-7 md:flex-none'
  return (
    <nav aria-label="Sekce přehledů" className={cn('flex rounded-lg bg-muted p-[3px] md:w-fit', className)}>
      <Link to="/a/$slug/reports" params={{ slug }} activeOptions={{ exact: true }} className={cls}>
        Přehled
      </Link>
      <Link to="/a/$slug/reports/vat" params={{ slug }} className={cls}>
        DPH
      </Link>
    </nav>
  )
}

/** Šipky ← hodnota → pro přepínání roku / období. */
export function Stepper({
  label,
  children,
  onPrev,
  onNext,
  nextDisabled,
  prevLabel,
  nextLabel,
}: {
  label: string
  children: ReactNode
  onPrev: () => void
  onNext: () => void
  nextDisabled?: boolean
  prevLabel: string
  nextLabel: string
}) {
  return (
    <div className="flex items-center gap-1" role="group" aria-label={label}>
      <Button variant="outline" size="icon" aria-label={prevLabel} onClick={onPrev}>
        <ChevronLeftIcon />
      </Button>
      <span className="min-w-12 flex-1 px-1 text-center font-medium tabular-nums md:flex-none" aria-live="polite">
        {children}
      </span>
      <Button variant="outline" size="icon" aria-label={nextLabel} disabled={nextDisabled} onClick={onNext}>
        <ChevronRightIcon />
      </Button>
    </div>
  )
}
