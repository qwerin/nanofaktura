import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

/** Výchozí „pending“ stav stránky (router defaultPendingComponent). */
export function PageSkeleton() {
  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-4 md:px-8 md:py-6" aria-busy="true" aria-label="Načítání">
      <Skeleton className="mb-6 h-8 w-48" />
      <ListSkeleton />
    </div>
  )
}

/** Skeleton seznamu — karty na mobilu, řádky tabulky na desktopu. */
export function ListSkeleton({ rows = 5, className }: { rows?: number; className?: string }) {
  return (
    <div className={cn('flex flex-col gap-2', className)} aria-hidden="true">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-3 rounded-xl border p-4 md:rounded-none md:border-x-0 md:border-t-0 md:px-2 md:py-3">
          <div className="flex flex-1 flex-col gap-2 md:flex-row md:items-center md:gap-6">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-4 w-40 md:flex-1" />
          </div>
          <Skeleton className="h-4 w-20" />
        </div>
      ))}
    </div>
  )
}

/** Skeleton formuláře. */
export function FormSkeleton({ fields = 6, className }: { fields?: number; className?: string }) {
  return (
    <div className={cn('flex flex-col gap-6', className)} aria-hidden="true">
      {Array.from({ length: fields }, (_, i) => (
        <div key={i} className="flex flex-col gap-2">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-11 w-full md:h-8" />
        </div>
      ))}
    </div>
  )
}
