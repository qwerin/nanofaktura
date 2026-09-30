import { useInfiniteQuery } from '@tanstack/react-query'
import { ChevronDownIcon, RotateCwIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { useRedeliverWebhook, webhookQueries } from '@/api/queries/webhooks'
import type { Webhook, WebhookDelivery } from '@/api/types'
import { useNow } from '@/components/bank/use-now'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/lib/date'
import { formatRelativeTime } from '@/lib/relative-time'
import { cn } from '@/lib/utils'
import { ResponseSnippet } from './webhook-dialogs'
import { httpStatusTone } from './webhook-events'

const statusStyles: Record<WebhookDelivery['status'], { label: string; className: string }> = {
  pending: { label: 'Čeká', className: 'bg-info/12 text-info dark:bg-info/20' },
  delivered: { label: 'Doručeno', className: 'bg-success/12 text-success dark:bg-success/20' },
  failed: { label: 'Selhalo', className: 'bg-destructive/10 text-destructive dark:bg-destructive/20' },
}

const toneText = {
  success: 'text-success',
  destructive: 'text-destructive',
  muted: 'text-muted-foreground',
} as const

/** Historie doručení webhooku (nejnovější první) s možností poslat znovu. */
export function DeliveriesDialog({ slug, webhook, onClose }: { slug: string; webhook: Webhook | null; onClose: () => void }) {
  return (
    <ResponsiveDialog
      open={webhook !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Doručení"
      description={webhook && <span className="break-all">{webhook.url}</span>}
      className="sm:max-w-2xl"
    >
      {webhook && <DeliveryList slug={slug} webhookId={webhook.id} />}
    </ResponsiveDialog>
  )
}

function DeliveryList({ slug, webhookId }: { slug: string; webhookId: number }) {
  const list = useInfiniteQuery(webhookQueries.deliveries(slug, webhookId))
  const now = useNow(30_000)
  if (list.isPending) {
    return (
      <div className="flex flex-col gap-2">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-14 w-full" />
        ))}
      </div>
    )
  }
  if (list.isError) return <p className="text-sm text-destructive">Doručení se nepodařilo načíst.</p>
  const items = list.data.pages.flatMap((p) => p.items)
  if (items.length === 0) return <p className="py-6 text-center text-sm text-muted-foreground">Zatím nic neodešlo. Zkuste „Poslat test“.</p>

  return (
    <div className="flex flex-col gap-3 md:max-h-[65vh] md:overflow-y-auto">
      <ul className="divide-y rounded-lg border">
        {items.map((d) => (
          <DeliveryRow key={d.id} slug={slug} delivery={d} now={now} />
        ))}
      </ul>
      {list.hasNextPage && (
        <Button variant="outline" className="self-center" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
          {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
          Načíst starší
        </Button>
      )}
    </div>
  )
}

function DeliveryRow({ slug, delivery: d, now }: { slug: string; delivery: WebhookDelivery; now: number }) {
  const [open, setOpen] = useState(false)
  const redeliver = useRedeliverWebhook(slug)
  const s = statusStyles[d.status]
  return (
    <li className="flex flex-col">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex min-h-14 w-full items-center gap-3 px-3 py-2 text-left hover:bg-muted/40 focus-visible:bg-muted/60 focus-visible:outline-none"
      >
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex flex-wrap items-center gap-2">
            <span className="truncate font-mono text-sm">{d.event}</span>
            <Badge variant="secondary" className={s.className}>
              {s.label}
            </Badge>
          </span>
          <span className="flex flex-wrap gap-x-2 text-xs text-muted-foreground">
            <time dateTime={d.created_at} title={formatDateTime(d.created_at)}>
              {formatRelativeTime(d.created_at, new Date(now))}
            </time>
            <span>
              · {d.attempts} {d.attempts === 1 ? 'pokus' : d.attempts >= 2 && d.attempts <= 4 ? 'pokusy' : 'pokusů'}
            </span>
            {d.status === 'pending' && d.next_attempt_at && <span>· další {formatDateTime(d.next_attempt_at)}</span>}
            {d.duration_ms > 0 && <span>· {d.duration_ms} ms</span>}
          </span>
        </div>
        <span className={cn('shrink-0 font-mono text-sm tabular-nums', toneText[httpStatusTone(d.response_status)])}>
          {d.response_status || (d.attempts > 0 ? '—' : '')}
        </span>
        <ChevronDownIcon className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="flex flex-col gap-3 px-3 pb-3">
          {d.error && <p className="rounded-lg bg-destructive/10 px-3 py-2 text-sm break-words text-destructive">{d.error}</p>}
          <ResponseSnippet body={prettyJSON(d.payload)} label="Odeslaný JSON" />
          <ResponseSnippet body={d.response_body} />
          <Button
            variant="outline"
            size="sm"
            className="self-start max-md:h-10"
            disabled={redeliver.isPending}
            onClick={() =>
              redeliver.mutate(
                { webhookId: d.webhook_id, deliveryId: d.id },
                { onSuccess: (r) => (r.status === 'delivered' ? toast.success('Znovu doručeno') : toast.warning('Odesláno, doručení se nezdařilo — zkusíme to znovu později')) },
              )
            }
          >
            {redeliver.isPending ? <Spinner data-icon="inline-start" /> : <RotateCwIcon data-icon="inline-start" />}
            Poslat znovu
          </Button>
        </div>
      )}
    </li>
  )
}

function prettyJSON(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}
