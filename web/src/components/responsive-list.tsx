import type { Key, ReactNode } from 'react'
import { ListSkeleton } from '@/components/skeletons'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

export interface ListColumn<T> {
  id: string
  header: ReactNode
  cell: (row: T) => ReactNode
  align?: 'left' | 'right'
  className?: string
}

interface ResponsiveListProps<T> {
  items: T[] | undefined
  getKey: (row: T) => Key
  /** Sloupce tabulky (desktop ≥ md). */
  columns: ListColumn<T>[]
  /** Obsah karty (mobil < md). Karta sama řeší rámeček, padding a klikatelnost. */
  renderCard: (row: T) => ReactNode
  onRowClick?: (row: T) => void
  isLoading?: boolean
  /** Zobrazí se, když `items` je prázdné pole. */
  empty?: ReactNode
  /** Pod seznamem — typicky tlačítko „Načíst další“. */
  footer?: ReactNode
  className?: string
}

/**
 * Seznam záznamů: karty na mobilu, tabulka na desktopu (SPEC §6).
 * Pro navigaci použijte `onRowClick` (např. `navigate({ to: …, params })`);
 * klávesnicí jde řádek/kartu aktivovat Enterem.
 */
export function ResponsiveList<T>({
  items,
  getKey,
  columns,
  renderCard,
  onRowClick,
  isLoading,
  empty,
  footer,
  className,
}: ResponsiveListProps<T>) {
  if (isLoading && !items) return <ListSkeleton className={className} />
  if (items && items.length === 0) return <>{empty}</>
  if (!items) return null

  const interactive = Boolean(onRowClick)
  const activate = (row: T) => onRowClick?.(row)

  return (
    <div className={className}>
      {/* Mobil: karty */}
      <ul className="flex flex-col gap-2 md:hidden">
        {items.map((row) => (
          <li key={getKey(row)}>
            {interactive ? (
              <button
                type="button"
                onClick={() => activate(row)}
                className="block w-full rounded-xl border bg-card p-4 text-left text-card-foreground transition-colors active:bg-muted"
              >
                {renderCard(row)}
              </button>
            ) : (
              <div className="rounded-xl border bg-card p-4 text-card-foreground">{renderCard(row)}</div>
            )}
          </li>
        ))}
      </ul>

      {/* Desktop: tabulka */}
      <div className="hidden overflow-hidden rounded-xl border bg-card md:block">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              {columns.map((c) => (
                <TableHead
                  key={c.id}
                  className={cn('h-10 px-4 text-xs font-medium text-muted-foreground', c.align === 'right' && 'text-right', c.className)}
                >
                  {c.header}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((row) => (
              <TableRow
                key={getKey(row)}
                tabIndex={interactive ? 0 : undefined}
                onClick={interactive ? () => activate(row) : undefined}
                onKeyDown={
                  interactive
                    ? (e) => {
                        if (e.key === 'Enter') activate(row)
                      }
                    : undefined
                }
                className={cn(interactive && 'cursor-pointer focus-visible:bg-muted/50 focus-visible:outline-none')}
              >
                {columns.map((c) => (
                  <TableCell
                    key={c.id}
                    className={cn('px-4 py-3', c.align === 'right' && 'text-right tabular-nums', c.className)}
                  >
                    {c.cell(row)}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {footer && <div className="mt-4 flex justify-center">{footer}</div>}
    </div>
  )
}
