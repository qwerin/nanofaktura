// Timeline událostí (GET /events): svislý seznam s ikonou podle rodiny, českým textem, autorem
// a relativním časem; „Načíst starší“. Na detailu dokladu/kontaktu jako sbalitelná sekce „Historie“.

import { useInfiniteQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { BotIcon, ChevronDownIcon, HistoryIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { eventQueries } from '@/api/queries/events'
import type { AppEvent, EventFilters } from '@/api/types'
import { useNow } from '@/components/bank/use-now'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/lib/date'
import { recordHref } from '@/lib/record-links'
import { formatRelativeTime } from '@/lib/relative-time'
import { cn } from '@/lib/utils'
import { eventLinksToRecord, eventVisual, toneClasses } from './event-meta'
import { HrefLink } from './href-link'

interface TimelineProps {
  filters: EventFilters
  perPage?: number
  /** Odkazovat na záznam události (aktivita účtu). Na vlastní timeline záznamu zbytečné. */
  linkRecords?: boolean
  /** Bez „Načíst starší“ (widget na přehledu). */
  noPaging?: boolean
  empty?: string
  className?: string
}

export function Timeline({ filters, perPage = 15, linkRecords, noPaging, empty = 'Zatím žádná aktivita.', className }: TimelineProps) {
  const { slug } = useParams({ from: '/a/$slug' })
  const query = useInfiniteQuery(eventQueries.list(slug, filters, perPage))
  const now = useNow()

  if (query.isPending) {
    return (
      <div className={cn('flex flex-col gap-3', className)} aria-busy="true">
        {[0, 1, 2].map((i) => (
          <div key={i} className="flex items-center gap-3">
            <Skeleton className="size-8 shrink-0 rounded-full" />
            <Skeleton className="h-9 flex-1" />
          </div>
        ))}
      </div>
    )
  }
  if (query.isError) return <p className={cn('text-sm text-destructive', className)}>Historii se nepodařilo načíst.</p>

  const items = query.data.pages.flatMap((p) => p.items)
  if (items.length === 0) return <p className={cn('text-sm text-muted-foreground', className)}>{empty}</p>

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <ol className="flex flex-col">
        {items.map((e, i) => (
          <TimelineItem
            key={e.id}
            event={e}
            last={i === items.length - 1}
            now={now}
            href={linkRecords && eventLinksToRecord(e.name) ? recordHref(slug, e.subject_type, e.subject_id) : null}
          />
        ))}
      </ol>
      {!noPaging && query.hasNextPage && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start max-md:h-10"
          disabled={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          {query.isFetchingNextPage && <Spinner data-icon="inline-start" />}
          Načíst starší
        </Button>
      )}
    </div>
  )
}

function TimelineItem({ event: e, last, now, href }: { event: AppEvent; last: boolean; now: number; href: string | null }) {
  const { icon: Icon, tone } = eventVisual(e.name)
  const text = e.text || e.name
  return (
    <li className="relative flex gap-3 pb-4 last:pb-0">
      {!last && <span aria-hidden="true" className="absolute top-9 bottom-1 left-4 w-px -translate-x-1/2 bg-border" />}
      <span className={cn('relative flex size-8 shrink-0 items-center justify-center rounded-full', toneClasses[tone])}>
        <Icon className="size-4" aria-hidden="true" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5 pt-1">
        {href ? (
          <HrefLink href={href} className="text-sm leading-snug break-words underline-offset-4 hover:underline">
            {text}
          </HrefLink>
        ) : (
          <p className="text-sm leading-snug break-words">{text}</p>
        )}
        <p className="flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
          {e.user_name ? (
            <span className="truncate">{e.user_name}</span>
          ) : (
            <span className="inline-flex items-center gap-1">
              <BotIcon className="size-3" aria-hidden="true" />
              Systém
            </span>
          )}
          <span aria-hidden="true">·</span>
          <time dateTime={e.created_at} title={formatDateTime(e.created_at)}>
            {formatRelativeTime(e.created_at, new Date(now))}
          </time>
        </p>
      </div>
    </li>
  )
}

/**
 * Sbalitelná sekce „Historie“ pro detail záznamu. Data se načtou až po rozbalení.
 * Použití: `<HistorySection subjectType="invoice" subjectId={inv.id} />`
 */
export function HistorySection({
  subjectType,
  subjectId,
  defaultOpen = false,
  className,
}: {
  subjectType: 'invoice' | 'expense' | 'subject' | 'price_item' | 'recurring'
  subjectId: number
  defaultOpen?: boolean
  className?: string
}) {
  const [open, setOpen] = useState(defaultOpen)
  const contentId = useId()
  return (
    <section className={cn('rounded-xl border bg-card text-card-foreground', className)}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={contentId}
        onClick={() => setOpen((o) => !o)}
        className="flex min-h-12 w-full items-center gap-2 rounded-xl px-4 py-3 text-left focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:px-5"
      >
        <HistoryIcon className="size-4 text-muted-foreground" aria-hidden="true" />
        <h2 className="flex-1 text-base font-semibold tracking-tight">Historie</h2>
        <ChevronDownIcon className={cn('size-4 text-muted-foreground transition-transform', open && 'rotate-180')} aria-hidden="true" />
      </button>
      {open && (
        <div id={contentId} className="px-4 pb-4 md:px-5 md:pb-5">
          <Timeline filters={{ subject_type: subjectType, subject_id: subjectId }} empty="Zatím žádné záznamy." />
        </div>
      )}
    </section>
  )
}
